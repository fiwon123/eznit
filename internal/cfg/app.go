package cfg

type AppCfg struct {
	port int
}

func NewAppCfg(port int) *AppCfg {
	return &AppCfg{}
}

func (cfg *AppCfg) Port() int {
	return cfg.port
}
