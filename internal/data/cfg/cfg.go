package cfg

type Data struct {
	port int
}

func New(port int) *Data {
	return &Data{
		port: port,
	}
}

func (cfg *Data) Port() int {
	return cfg.port
}
