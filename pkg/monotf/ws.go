package monotf

import (
	"fmt"
	"strings"

	"github.com/robertlestak/monotf/internal/db"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"
)

const (
	WorkspaceStatusApplied WorkspaceStatus = "applied"
	WorkspaceStatusFailed  WorkspaceStatus = "failed"
	WorkspaceStatusPending WorkspaceStatus = "pending"
	WorkspaceStatusUnknown WorkspaceStatus = "unknown"
	WorkspaceStatusDrifted WorkspaceStatus = "drifted"
)

var (
	WorkspaceStatuses = []WorkspaceStatus{
		WorkspaceStatusApplied,
		WorkspaceStatusFailed,
		WorkspaceStatusPending,
		WorkspaceStatusUnknown,
		WorkspaceStatusDrifted,
	}
)

type WorkspaceStatus string

type OrgStatusCounts struct {
	Org    string                  `json:"org"`
	Counts map[WorkspaceStatus]int `json:"counts"`
}

func (w *Workspace) EnsureValidStatus() error {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "EnsureValidStatus",
		"org":    w.Org,
		"ws":     w.Name,
		"status": w.Status,
	})
	l.Debug("start")
	if w.Status == "" {
		w.Status = WorkspaceStatusUnknown
		l.Debug("end")
		return nil
	}
	for _, s := range WorkspaceStatuses {
		if s == w.Status {
			l.Debug("end")
			return nil
		}
	}
	l.Error("invalid status")
	return fmt.Errorf("invalid status")
}

func (w *Workspace) Save() error {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "Save",
		"org":    w.Org,
		"ws":     w.Name,
		"status": w.Status,
	})
	l.Debug("start")
	if w.Org == "" || w.Name == "" {
		l.Error("org or name is empty")
		return fmt.Errorf("org or name is empty")
	}
	if err := w.EnsureValidStatus(); err != nil {
		l.WithError(err).Error("invalid status")
		return err
	}
	// if w.Running is nil, then use the value from the db
	if w.Running == nil {
		var isRunning bool
		if err := db.DB.Model(&Workspace{}).Where("org = ? AND name = ?", w.Org, w.Name).Pluck("running", &isRunning).Error; err != nil {
			l.WithError(err).Error("failed to get running")
			return err
		}
		w.Running = &isRunning
	}
	// get current lock id from db
	existingLockID := ""
	if err := db.DB.Model(&Workspace{}).Where("org = ? AND name = ?", w.Org, w.Name).Pluck("lock_id", &existingLockID).Error; err != nil {
		l.WithError(err).Error("failed to get lock id")
		return err
	}
	// if lock id is different, throw error unless force is set
	if w.LockId != nil && existingLockID != "" && existingLockID != *w.LockId && !w.Force {
		l.Errorf("lock id mismatch. existing: %s, new: %s", existingLockID, *w.LockId)
		return fmt.Errorf("lock id mismatch. existing: %s, new: %s", existingLockID, *w.LockId)
	}
	if err := db.DB.Clauses(clause.OnConflict{
		// update all
		Columns:   []clause.Column{{Name: "org"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "output", "running", "version", "path", "lock_id"}),
	}).Save(w).Error; err != nil {
		l.WithError(err).Error("failed to save workspace")
		return err
	}
	l.Debug("end")
	return nil
}

func (w *Workspace) Delete() error {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "Delete",
		"org":    w.Org,
		"ws":     w.Name,
		"status": w.Status,
	})
	l.Debug("start")
	// if id is not set, then we need to get it
	if w.ID == 0 {
		if err := w.Get(); err != nil {
			l.WithError(err).Error("failed to get workspace")
			return err
		}
	}
	if err := db.DB.Delete(w).Error; err != nil {
		l.WithError(err).Error("failed to delete workspace")
		return err
	}
	l.Debug("end")
	return nil
}

func (w *Workspace) InferStateFromOutput() error {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "InferStateFromOutput",
		"org": w.Org,
		"ws":  w.Name,
	})
	l.Debug("start")
	if w.Output == "" {
		l.Error("output is empty")
		return fmt.Errorf("output is empty")
	}
	// if output contains "Error: ", then we failed
	// if output contains "Your infrastructure matches the configuration.", then we applied
	// if output contains "No changes. Infrastructure is up-to-date.", then we applied
	// if output contains "Plan: 0 to add, 0 to change, 0 to destroy.", then we applied
	// if output contains "Terraform will perform the following actions", then we are pending
	if strings.Contains(w.Output, "Error:") {
		w.Status = WorkspaceStatusFailed
	} else if strings.Contains(w.Output, "Your infrastructure matches the configuration.") {
		w.Status = WorkspaceStatusApplied
	} else if strings.Contains(w.Output, "Apply complete") {
		w.Status = WorkspaceStatusApplied
	} else if strings.Contains(w.Output, "No changes. Infrastructure is up-to-date.") {
		w.Status = WorkspaceStatusApplied
	} else if strings.Contains(w.Output, "Plan: 0 to add, 0 to change, 0 to destroy.") {
		w.Status = WorkspaceStatusApplied
	} else if strings.Contains(w.Output, "Terraform will perform the following actions") {
		w.Status = WorkspaceStatusPending
	} else {
		w.Status = WorkspaceStatusUnknown
	}
	l.Debug("end")
	return nil
}

func (w *Workspace) Get() error {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "Get",
		"org":    w.Org,
		"ws":     w.Name,
		"status": w.Status,
	})
	l.Debug("start")
	if w.Org == "" || w.Name == "" {
		l.Error("org or name is empty")
		return fmt.Errorf("org or name is empty")
	}
	if err := db.DB.Where("org = ? AND name = ?", w.Org, w.Name).First(w).Error; err != nil {
		l.WithError(err).Error("failed to get workspace")
		return err
	}
	l.Debug("end")
	return nil
}

func ListOrgWorkspaces(org string) ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "ListOrgWorkspaces",
		"org": org,
	})
	l.Debug("start")
	if org == "" {
		l.Error("org is empty")
		return nil, fmt.Errorf("org is empty")
	}
	var ws []Workspace
	if err := db.DB.Where("org = ?", org).Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func ListAllWorkspaces() ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "ListAllWorkspaces",
	})
	l.Debug("start")
	var ws []Workspace
	if err := db.DB.Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func ListOrgWorkspacesLike(org, like string) ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg":  "ws",
		"fn":   "ListOrgWorkspacesLike",
		"org":  org,
		"like": like,
	})
	l.Debug("start")
	if org == "" {
		l.Error("org is empty")
		return nil, fmt.Errorf("org is empty")
	}
	var ws []Workspace
	if err := db.DB.Where("org = ? AND name LIKE ?", org, like).Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func ListAllWorkspacesLike(orgLike, nameLike string) ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg":  "ws",
		"fn":   "ListAllWorkspacesLike",
		"org":  orgLike,
		"like": nameLike,
	})
	l.Debug("start")
	var ws []Workspace
	if err := db.DB.Where("org LIKE ? AND name LIKE ?", orgLike, nameLike).Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func ListOrgs() ([]string, error) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "ListOrgs",
	})
	l.Debug("start")
	var orgs []string
	if err := db.DB.Model(&Workspace{}).Distinct().Pluck("org", &orgs).Error; err != nil {
		l.WithError(err).Error("failed to list orgs")
		return nil, err
	}
	l.Debug("end")
	return orgs, nil
}

func ListValidStatuses() []WorkspaceStatus {
	return WorkspaceStatuses
}

func ListOrgWorkspaceByStatus(org string, status WorkspaceStatus) ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "ListOrgWorkspaceByStatus",
		"org":    org,
		"status": status,
	})
	l.Debug("start")
	if org == "" {
		l.Error("org is empty")
		return nil, fmt.Errorf("org is empty")
	}
	var ws []Workspace
	if err := db.DB.Where("org = ? AND status = ?", org, status).Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func ListAllWorkspaceByStatus(status WorkspaceStatus) ([]Workspace, error) {
	l := log.WithFields(log.Fields{
		"pkg":    "ws",
		"fn":     "ListAllWorkspaceByStatus",
		"status": status,
	})
	l.Debug("start")
	var ws []Workspace
	if err := db.DB.Where("status = ?", status).Find(&ws).Error; err != nil {
		l.WithError(err).Error("failed to list workspaces")
		return nil, err
	}
	l.Debug("end")
	return ws, nil
}

func GetOrgStatusCount(org string) (OrgStatusCounts, error) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "GetOrgStatusCount",
		"org": org,
	})
	l.Debug("start")
	if org == "" {
		l.Error("org is empty")
		return OrgStatusCounts{}, fmt.Errorf("org is empty")
	}
	var c OrgStatusCounts
	c.Counts = make(map[WorkspaceStatus]int)
	c.Org = org
	type wsCount struct {
		Status WorkspaceStatus
		Count  int
	}
	var counts []wsCount
	if err := db.DB.Table("workspaces").Select("status, count(*)").Where("org = ?", org).Group("status").Scan(&counts).Error; err != nil {
		l.WithError(err).Error("failed to get org status count")
		return OrgStatusCounts{}, err
	}
	l.Debug("end")
	for _, v := range counts {
		c.Counts[v.Status] = v.Count
	}
	// set rest of the counts to 0
	for _, s := range WorkspaceStatuses {
		if _, ok := c.Counts[s]; !ok {
			c.Counts[s] = 0
		}
	}
	return c, nil
}

func GetAllStatusCount() ([]OrgStatusCounts, error) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "GetAllStatusCount",
	})
	l.Debug("start")
	var counts []OrgStatusCounts
	var orgs []string
	if err := db.DB.Model(&Workspace{}).Distinct().Pluck("org", &orgs).Error; err != nil {
		l.WithError(err).Error("failed to list orgs")
		return nil, err
	}
	for _, org := range orgs {
		c, err := GetOrgStatusCount(org)
		if err != nil {
			l.WithError(err).Error("failed to get org status count")
			return nil, err
		}
		counts = append(counts, c)
	}
	l.Debug("end")
	return counts, nil
}
