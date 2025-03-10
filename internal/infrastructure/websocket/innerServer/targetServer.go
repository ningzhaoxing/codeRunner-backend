package innerServer

type TargetServer struct {
	host string
	port string
	path string
}

func NewTargetServer(host, port, path string) *TargetServer {
	return &TargetServer{
		host: host,
		port: port,
		path: path,
	}
}
