package innerServer

type TargetServer struct {
	host     string
	port     string
	path     string
	rowQuery string
}

func NewTargetServer(host, port, path, rowQuery string) *TargetServer {
	return &TargetServer{
		host:     host,
		port:     port,
		path:     path,
		rowQuery: rowQuery,
	}
}
