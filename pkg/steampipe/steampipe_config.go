package steampipe

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/opengovern/og-util/pkg/config"
)

func PopulateSteampipeConfig(elasticSearchConfig config.ElasticSearch, steampipePluginName string) error {
	err := BuildSpecFile(steampipePluginName, elasticSearchConfig)
	if err != nil {
		return err
	}
	err = PopulateEnv(elasticSearchConfig)
	if err != nil {
		return err
	}

	return nil
}

func PopulateOpenGovernancePluginSteampipeConfig(elasticSearchConfig config.ElasticSearch, postgresConfig config.Postgres) error {
	if len(postgresConfig.SSLMode) == 0 {
		postgresConfig.SSLMode = "disable"
	}

	content := `
connection "opengovernance" {
  plugin = "local/opengovernance"
  addresses = ["` + elasticSearchConfig.Address + `"]
  username = "` + elasticSearchConfig.Username + `"
  password = "` + elasticSearchConfig.Password + `"
  pg_host = "` + postgresConfig.Host + `"
  pg_port = "` + postgresConfig.Port + `"
  pg_user = "` + postgresConfig.Username + `"
  pg_password = "` + postgresConfig.Password + `"
  pg_database = "` + postgresConfig.DB + `"
  pg_ssl_mode = "` + postgresConfig.SSLMode + `"
}
`
	dirname, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	filePath := path.Join(dirname, ".steampipe", "config", "opengovernance.spc")
	os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	err = os.WriteFile(filePath, []byte(content), os.ModePerm)
	if err != nil {
		return err
	}

	err = os.Setenv("STEAMPIPE_CACHE", "false")
	if err != nil {
		return err
	}
	err = os.Setenv("STEAMPIPE_MEMORY_MAX_MB", "4096")
	if err != nil {
		return err
	}
	err = os.Setenv("ELASTICSEARCH_ADDRESS", elasticSearchConfig.Address)
	if err != nil {
		return err
	}
	err = os.Setenv("ELASTICSEARCH_USERNAME", elasticSearchConfig.Username)
	if err != nil {
		return err
	}
	err = os.Setenv("ELASTICSEARCH_PASSWORD", elasticSearchConfig.Password)
	if err != nil {
		return err
	}

	return nil
}

func PopulateEnv(config config.ElasticSearch) error {
	err := os.Setenv("ELASTICSEARCH_ADDRESS", config.Address)
	if err != nil {
		return err
	}
	err = os.Setenv("ELASTICSEARCH_USERNAME", config.Username)
	if err != nil {
		return err
	}
	err = os.Setenv("ELASTICSEARCH_PASSWORD", config.Password)
	if err != nil {
		return err
	}
	return nil
}

func BuildSpecFile(plugin string, config config.ElasticSearch) error {
	content := `
plugin "` + plugin + `" {
  memory_max_mb = 4096 # megabytes
}
connection "` + plugin + `" {
  plugin = "` + plugin + `"
  addresses = ["` + config.Address + `"]
  username = "` + config.Username + `"
  password = "` + config.Password + `"
}
`
	dirname, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	filePath := path.Join(dirname, ".steampipe", "config", plugin+".spc")
	os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	return os.WriteFile(filePath, []byte(content), os.ModePerm)
}

// StartSteampipeServiceAndGetConnection starts steampipe service and returns steampipe connection
// NOTE: this function will only work on images that have steampipe installed & the PopulateSteampipeConfig is called beforehand
func StartSteampipeServiceAndGetConnection(logger *zap.Logger) (*Database, error) {
	err := os.Setenv("STEAMPIPE_CACHE", "false")
	if err != nil {
		return nil, err
	}
	err = os.Setenv("STEAMPIPE_UPDATE_CHECK", "false")
	if err != nil {
		return nil, err
	}
	defaultSpc := `
options "database" {
  port               = 9193                  # any valid, open port number
  listen             = "local"               # local (alias for localhost), network (alias for *), or a comma separated list of hosts and/or IP addresses , or any valid combination of hosts and/or IP addresses
  start_timeout      = 30                    # maximum time (in seconds) to wait for the database to start up
  cache              = false                  # true, false
  cache_max_ttl      = 1                   # max expiration (TTL) in seconds
  cache_max_size_mb  = 1                  # max total size of cache across all plugins
}
`
	dirname, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	filePath := path.Join(dirname, ".steampipe", "config", "default.spc")
	os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	err = os.WriteFile(filePath, []byte(defaultSpc), os.ModePerm)
	if err != nil {
		return nil, err
	}

	for retry := 0; retry < 5; retry++ {
		cmd := exec.Command("steampipe", "plugin", "list")
		cmdOut, err := cmd.Output()
		if err != nil {
			logger.Error("pre service init plugin list failed", zap.Error(err), zap.String("body", string(cmdOut)))
			time.Sleep(5 * time.Second)
			if retry == 4 {
				return nil, err
			}
			continue
		}
		logger.Info("pre service init plugin list succeeded", zap.String("body", string(cmdOut)))
		break
	}

	cmd := exec.Command("steampipe", "service", "stop", "--force")
	err = cmd.Start()
	if err != nil {
		logger.Error("first stop failed", zap.Error(err))
		return nil, err
	}
	time.Sleep(5 * time.Second)
	//NOTE: stop must be called twice. it's not a mistake
	cmd = exec.Command("steampipe", "service", "stop", "--force")
	err = cmd.Start()
	if err != nil {
		logger.Error("second stop failed", zap.Error(err))
		return nil, err
	}
	time.Sleep(5 * time.Second)

	cmd = exec.Command("steampipe", "service", "start", "--database-password", "abcd")
	cmdOut, err := cmd.Output()
	if err != nil {
		logger.Error("start failed", zap.Error(err), zap.String("body", string(cmdOut)))
		return nil, err
	}
	time.Sleep(5 * time.Second)

	logger.Info("steampipe service started")

	steampipeConn, err := NewSteampipeDatabase(GetDefaultSteampipeOption())
	if err != nil {
		return nil, err
	}

	logger.Info("steampipe database created")
	return steampipeConn, nil
}

func StopSteampipeService(logger *zap.Logger) error {
	for i := 0; i < 5; i++ {
		cmd := exec.Command("steampipe", "service", "stop", "--force")
		err := cmd.Start()
		if err != nil {
			logger.Error("first stop failed", zap.Error(err))
			return err
		}
		time.Sleep(time.Second)
	}
	return nil
}
