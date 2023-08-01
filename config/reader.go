package config

import (
	"github.com/spf13/viper"
)

type ProductConfig struct {
	TFVars  string `mapstructure:"ENV_TFVARS"`
	Product string `mapstructure:"ENV_PRODUCT"`
}

func LoadConfigEnv(path string) (config ProductConfig, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}
	err = viper.Unmarshal(&config)

	return
}
