package bananceStrategy

type LoadBalance interface {
	Add(int, int64)
	Get() (int, error)
	Remove(int)
	UpdateWeight(int, int64)
}
