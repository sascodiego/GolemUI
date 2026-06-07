package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type ConfigConexion struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

type BootstrapConfig struct {
	UIDB             ConfigConexion `mapstructure:"uidb"`
	BusinessDB       ConfigConexion `mapstructure:"business_db"`
	EntryPointViewID string         `mapstructure:"entry_point_view_id"`
	LayoutQuery      string         `mapstructure:"layout_query"`
}

func validateConexion(c ConfigConexion, name string) error {
	if c.Host == "" && c.Port == 0 && c.Database == "" && c.User == "" && c.Password == "" {
		return fmt.Errorf("sub-table %s not found or invalid", name)
	}
	if c.Host == "" || c.Port == 0 || c.Database == "" || c.User == "" {
		return fmt.Errorf("missing required connection fields in %s", name)
	}
	return nil
}

func LoadConfig(path string) (*BootstrapConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", path)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg BootstrapConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validateConexion(cfg.UIDB, "UIDB"); err != nil {
		return nil, err
	}
	if err := validateConexion(cfg.BusinessDB, "BusinessDB"); err != nil {
		return nil, err
	}

	return &cfg, nil
}
