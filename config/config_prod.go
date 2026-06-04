//go:build !dev

package config

func Load() *Config {
	cfg := loadBase()
	if cfg == nil {
		return nil
	}
	cfg.Environment = Prod
	return cfg
}
