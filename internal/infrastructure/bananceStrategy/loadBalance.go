package bananceStrategy

type LoadBalance interface {
	Add(*WeightNode)
	Get() (*WeightNode, error)
	Remove(string)
	UpdateWeight(string, int64)
}
