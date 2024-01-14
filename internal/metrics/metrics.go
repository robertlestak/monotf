package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	OrgStatusSummary = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "monotf_org_status_summary",
		Help: "Count of workspace statuses by organization",
	}, []string{"org", "status"})
	WorkspaceStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "monotf_workspace_status",
		Help: "Workspace status by organization, workspace, and version",
	}, []string{"org", "workspace", "version", "status"})
	WorkspaceLastRun = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "monotf_workspace_last_run",
		Help: "Last run time of workspace",
	}, []string{"org", "workspace"})
	WorkspaceRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "monotf_workspace_running",
		Help: "Workspace is running",
	}, []string{"org", "workspace"})
)

func Init() {
	prometheus.MustRegister(OrgStatusSummary)
	prometheus.MustRegister(WorkspaceStatus)
	prometheus.MustRegister(WorkspaceLastRun)
	prometheus.MustRegister(WorkspaceRunning)
}
