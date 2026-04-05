package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// CodeExecutionTotal 各语言执行次数，按结果状态（success / error）细分
	CodeExecutionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "code_execution_total",
			Help: "Total number of code executions, labeled by language and status.",
		},
		[]string{"language", "status"},
	)

	// CodeExecutionDuration 各语言执行耗时（秒），仅统计成功执行
	CodeExecutionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "code_execution_duration_seconds",
			Help:    "Duration of successful code executions in seconds, labeled by language.",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"language"},
	)

	// WSClientsConnected 当前已连接的 Worker 客户端数量
	WSClientsConnected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_clients_connected",
			Help: "Number of currently connected WebSocket worker clients.",
		},
	)

	// PoolIdleGauge 各语言容器池空闲容器数
	PoolIdleGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_pool_idle_count",
			Help: "Number of idle containers in pool, labeled by language.",
		},
		[]string{"language"},
	)

	// PoolAcquireDuration 获取容器的等待时间
	PoolAcquireDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "container_pool_acquire_duration_seconds",
			Help:    "Duration of acquiring a container from pool.",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 5},
		},
		[]string{"language"},
	)

	// PoolReplenishTotal 容器重建计数
	PoolReplenishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "container_pool_replenish_total",
			Help: "Total container replenish attempts, labeled by language and result.",
		},
		[]string{"language", "result"},
	)
)

func init() {
	prometheus.MustRegister(
		CodeExecutionTotal,
		CodeExecutionDuration,
		WSClientsConnected,
		PoolIdleGauge,
		PoolAcquireDuration,
		PoolReplenishTotal,
	)
}
