package dogeboxd

// handle jobs on behalf of Dogeboxd and
// return them via it's own update channel.
type SystemUpdater interface {
	AddJob(Job)
	GetUpdateChannel() chan Job
}

type SystemMonitor interface {
	GetMonChannel() chan []string
	GetStatChannel() chan map[string]ProcStatus
}

// SystemMonitor issues these for monitored PUPs
type ProcStatus struct {
	CPUPercent float64
	MEMPercent float64
	MEMMb      float64
	Running    bool
}
