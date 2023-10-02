package steampipe

import (
	"errors"
	"os"
	"path"
	"path/filepath"

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
