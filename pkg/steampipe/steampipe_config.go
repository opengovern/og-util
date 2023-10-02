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
	connector source.Type, AccountID string,
	encodedResourceGroupFilter *string) error {
	switch connector {
	case source.CloudAWS:
		err := BuildSpecFile("aws", elasticSearchConfig, AccountID, encodedResourceGroupFilter)
		if err != nil {
			return err
		}

		err = PopulateEnv(elasticSearchConfig, AccountID)
		if err != nil {
			return err
		}
	case source.CloudAzure:
		err := BuildSpecFile("azure", elasticSearchConfig, AccountID, encodedResourceGroupFilter)
		if err != nil {
			return err
		}

		err = BuildSpecFile("azuread", elasticSearchConfig, AccountID, encodedResourceGroupFilter)
		if err != nil {
			return err
		}

		err = PopulateEnv(elasticSearchConfig, AccountID)
		if err != nil {
			return err
		}
	default:
		return errors.New("error: invalid source type")
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
  encoded_resource_group_filters = "` + ergf + `"
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
	err := cmd.Start()
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

	cmd = exec.Command("steampipe", "service", "start", "--database-listen", "network", "--database-port",
		"9193", "--database-password", "abcd")
	cmdOut, err := cmd.Output()
	if err != nil {
		logger.Error("start failed", zap.Error(err), zap.String("body", string(cmdOut)))
		return nil, err
	}
	time.Sleep(5 * time.Second)

	logger.Info("steampipe service started")

	for retry := 0; retry < 5; retry++ {
		cmd := exec.Command("steampipe", "plugin", "list")
		cmdOut, err := cmd.Output()
		if err != nil {
			logger.Error("post service init plugin list failed", zap.Error(err), zap.String("body", string(cmdOut)))
			time.Sleep(5 * time.Second)
			if retry == 4 {
				return nil, err
			}
			continue
		}
		logger.Info("post service init plugin list succeeded", zap.String("body", string(cmdOut)))
		break
	}

	steampipeConn, err := NewSteampipeDatabase(Option{
		Host: "localhost",
		Port: "9193",
		User: "steampipe",
		Pass: "abcd",
		Db:   "steampipe",
	})
	if err != nil {
		return nil, err
	}

	logger.Info("steampipe database created")
	return steampipeConn, nil
}
