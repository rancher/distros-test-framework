package config

import (
	"sync"

	"github.com/spf13/viper"
)

var (
	product *ProductConfig
	once    sync.Once
)

type ProductConfig struct {
	TFVars  string `mapstructure:"ENV_TFVARS"`
	Product string `mapstructure:"ENV_PRODUCT"`
}

// AddConfigEnv returns a singleton of ProductConfig from yaml config file
func AddConfigEnv(path string) (*ProductConfig, error) {
	once.Do(func() {
		var err error
		product, err = loadConfigEnv(path)
		if err != nil {
			return
		}
	})

	return product, nil
}

func loadConfigEnv(path string) (config *ProductConfig, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return nil, err

	}
	err = viper.Unmarshal(&config)

	return config, err
}
