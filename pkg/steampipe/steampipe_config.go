package steampipe

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

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
  encodedResourceGroupFilter = "` + ergf + `"
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
