package steampipe

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kuberTypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kaytu-io/kaytu-util/pkg/config"
	"github.com/kaytu-io/kaytu-util/pkg/source"
)

func PopulateSteampipeConfig(elasticSearchConfig config.ElasticSearch,
	connector source.Type, accountID string,
	encodedResourceCollectionFilter *string) error {
	switch connector {
	case source.CloudAWS:
		err := BuildSpecFile("aws", elasticSearchConfig, accountID, encodedResourceCollectionFilter)
		if err != nil {
			return err
		}

		err = PopulateEnv(elasticSearchConfig, accountID)
		if err != nil {
			return err
		}
	case source.CloudAzure:
		err := BuildSpecFile("azure", elasticSearchConfig, accountID, encodedResourceCollectionFilter)
		if err != nil {
			return err
		}

		err = BuildSpecFile("azuread", elasticSearchConfig, accountID, encodedResourceCollectionFilter)
		if err != nil {
			return err
		}

		err = PopulateEnv(elasticSearchConfig, accountID)
		if err != nil {
			return err
		}
	default:
		return errors.New("error: invalid source type")
	}
	return nil
}

func PopulateKaytuPluginSteampipeConfig(elasticSearchConfig config.ElasticSearch, postgresConfig config.Postgres,
	encodedResourceCollectionFilter *string) error {

	ergf := ""
	if encodedResourceCollectionFilter != nil {
		ergf = *encodedResourceCollectionFilter
	}

	if len(postgresConfig.SSLMode) == 0 {
		postgresConfig.SSLMode = "disable"
	}

	content := `
connection "kaytu" {
  plugin = "local/kaytu"
  addresses = ["` + elasticSearchConfig.Address + `"]
  username = "` + elasticSearchConfig.Username + `"
  password = "` + elasticSearchConfig.Password + `"
  encoded_resource_collection_filters = "` + ergf + `"
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

	filePath := path.Join(dirname, ".steampipe", "config", "kaytu.spc")
	os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	err = os.WriteFile(filePath, []byte(content), os.ModePerm)
	if err != nil {
		return err
	}

	err = os.Setenv("STEAMPIPE_CACHE", "false")
	if err != nil {
		return err
	}
	err = os.Setenv("ES_ADDRESS", elasticSearchConfig.Address)
	if err != nil {
		return err
	}
	err = os.Setenv("ES_USERNAME", elasticSearchConfig.Username)
	if err != nil {
		return err
	}
	err = os.Setenv("ES_PASSWORD", elasticSearchConfig.Password)
	if err != nil {
		return err
	}

	return nil
}

func PopulateEnv(config config.ElasticSearch, accountID string) error {
	err := os.Setenv("STEAMPIPE_ACCOUNT_ID", accountID)
	if err != nil {
		return err
	}
	err = os.Setenv("ES_ADDRESS", config.Address)
	if err != nil {
		return err
	}
	err = os.Setenv("ES_USERNAME", config.Username)
	if err != nil {
		return err
	}
	err = os.Setenv("ES_PASSWORD", config.Password)
	if err != nil {
		return err
	}
	return nil
}

func BuildSpecFile(plugin string, config config.ElasticSearch,
	accountID string,
	encodedResourceGroupFilter *string) error {

	ergf := ""
	if encodedResourceGroupFilter != nil {
		ergf = *encodedResourceGroupFilter
	}

	content := `
connection "` + plugin + `" {
  plugin = "` + plugin + `"
  addresses = ["` + config.Address + `"]
  username = "` + config.Username + `"
  password = "` + config.Password + `"
  accountID = "` + accountID + `"
  encoded_resource_collection_filters = "` + ergf + `"
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

func GetStackElasticConfig(workspaceId string, stackId string) (config.ElasticSearch, error) {
	scheme := runtime.NewScheme()
	if err := helmv2.AddToScheme(scheme); err != nil {
		return config.ElasticSearch{}, err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return config.ElasticSearch{}, err
	}
	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return config.ElasticSearch{}, err
	}

	releaseName := stackId
	secretName := fmt.Sprintf("%s-es-elastic-user", releaseName)

	secret := &corev1.Secret{}
	err = kubeClient.Get(context.TODO(), kuberTypes.NamespacedName{
		Namespace: workspaceId,
		Name:      secretName,
	}, secret)
	if err != nil {
		return config.ElasticSearch{}, err
	}
	password, err := base64.URLEncoding.DecodeString(string(secret.Data["elastic"]))
	if err != nil {
		return config.ElasticSearch{}, err
	}
	return config.ElasticSearch{
		Address:  fmt.Sprintf("https://%s-es-http:9200/", releaseName),
		Username: "elastic",
		Password: string(password),
	}, nil
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
