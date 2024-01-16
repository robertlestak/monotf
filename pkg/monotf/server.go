package monotf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robertlestak/monotf/internal/db"
	"github.com/robertlestak/monotf/internal/metrics"
	log "github.com/sirupsen/logrus"
)

func HandleSaveWorkspace(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleSaveWorkspace",
	})
	l.Debug("start")
	var ws Workspace
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		l.WithError(err).Error("failed to decode request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// if output is base64 encoded, decode
	if ws.Output != "" {
		decoded, err := base64.StdEncoding.DecodeString(ws.Output)
		if err != nil {
			l.WithError(err).Debug("failed to decode base64 output")
			// assume it's not base64 encoded
		} else {
			ws.Output = string(decoded)
		}
	}
	if ws.Status == "" && ws.Output != "" {
		ws.Status = "unknown"
		if err := ws.InferStateFromOutput(); err != nil {
			l.WithError(err).Error("failed to infer state from output")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err.Error())
			return
		}
	}
	if err := ws.Save(); err != nil {
		l.WithError(err).Error("failed to save workspace")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	l.Debug("end")
}

func HandleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleDeleteWorkspace",
	})
	l.Debug("start")
	var ws Workspace
	vars := mux.Vars(r)
	ws.Org = vars["org"]
	ws.Name = vars["name"]
	if err := ws.Delete(); err != nil {
		l.WithError(err).Error("failed to delete workspace")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	l.Debug("end")
}

func HandleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleGetWorkspace",
	})
	l.Debug("start")
	var ws Workspace
	vars := mux.Vars(r)
	ws.Org = vars["org"]
	ws.Name = vars["name"]
	if err := ws.Get(); err != nil {
		// if the workspace doesn't exist, create it
		if err := ws.Save(); err != nil {
			l.WithError(err).Error("failed to save workspace")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err.Error())
			return
		}
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	l.Debug("end")
}

func HandleListOrgWorkspaces(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListOrgWorkspaces",
	})
	l.Debug("start")
	vars := mux.Vars(r)
	org := vars["org"]
	ws, err := ListOrgWorkspaces(org)
	if err != nil {
		l.WithError(err).Error("failed to list org workspaces")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListAllWorkspaces(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListAllWorkspaces",
	})
	l.Debug("start")
	ws, err := ListAllWorkspaces()
	if err != nil {
		l.WithError(err).Error("failed to list all workspaces")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

type ListOrgWorkspacesLikeRequest struct {
	Org  string `json:"org"`
	Like string `json:"like"`
}

func HandleListOrgWorkspacesLike(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListOrgWorkspacesLike",
	})
	l.Debug("start")
	var req ListOrgWorkspacesLikeRequest
	req.Org = r.FormValue("org")
	req.Like = r.FormValue("like")
	ws, err := ListOrgWorkspacesLike(req.Org, req.Like)
	if err != nil {
		l.WithError(err).Error("failed to list org workspaces like")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListAllWorkspacesLike(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListAllWorkspacesLike",
	})
	l.Debug("start")
	var req ListOrgWorkspacesLikeRequest
	req.Org = r.FormValue("org")
	req.Like = r.FormValue("like")
	ws, err := ListAllWorkspacesLike(req.Org, req.Like)
	if err != nil {
		l.WithError(err).Error("failed to list all workspaces like")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListOrgs(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListOrgs",
	})
	l.Debug("start")
	orgs, err := ListOrgs()
	if err != nil {
		l.WithError(err).Error("failed to list orgs")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(orgs); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListValidStatuses(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListValidStatuses",
	})
	l.Debug("start")
	if err := json.NewEncoder(w).Encode(ListValidStatuses()); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListOrgWorkspacesByStatus(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListOrgWorkspacesByStatus",
	})
	l.Debug("start")
	vars := mux.Vars(r)
	org := vars["org"]
	status := vars["status"]
	stat := WorkspaceStatus(status)
	ws, err := ListOrgWorkspaceByStatus(org, stat)
	if err != nil {
		l.WithError(err).Error("failed to list org workspaces by status")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleListAllWorkspacesByStatus(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleListAllWorkspacesByStatus",
	})
	l.Debug("start")
	vars := mux.Vars(r)
	status := vars["status"]
	stat := WorkspaceStatus(status)
	ws, err := ListAllWorkspaceByStatus(stat)
	if err != nil {
		l.WithError(err).Error("failed to list all workspaces by status")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(ws); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleOrgStatusCount(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleOrgStatusCount",
	})
	l.Debug("start")
	vars := mux.Vars(r)
	org := vars["org"]
	counts, err := GetOrgStatusCount(org)
	if err != nil {
		l.WithError(err).Error("failed to get org status count")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(counts); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func HandleAllStatusCount(w http.ResponseWriter, r *http.Request) {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "HandleAllStatusCount",
	})
	l.Debug("start")
	counts, err := GetAllStatusCount()
	if err != nil {
		l.WithError(err).Error("failed to get all status count")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	if err := json.NewEncoder(w).Encode(counts); err != nil {
		l.WithError(err).Error("failed to encode response body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	l.Debug("end")
}

func getMetrics() error {
	l := log.WithField("func", "getMetrics")
	l.Debug("getting metrics")
	go func() {
		counts, err := GetAllStatusCount()
		if err != nil {
			l.WithError(err).Error("error getting all status count")
			return
		}
		for _, c := range counts {
			for status, cv := range c.Counts {
				metrics.OrgStatusSummary.WithLabelValues(c.Org, string(status)).Set(float64(cv))
			}
		}
	}()
	go func() {
		ws, err := ListAllWorkspaces()
		if err != nil {
			l.WithError(err).Error("error getting all workspaces")
			return
		}
		for _, w := range ws {
			metrics.WorkspaceStatus.WithLabelValues(w.Org, w.Name, w.Version, string(w.Status)).Set(1)
			metrics.WorkspaceLastRun.WithLabelValues(w.Org, w.Name).Set(float64(w.UpdatedAt.Unix()))
			if w.Running != nil && *w.Running {
				metrics.WorkspaceRunning.WithLabelValues(w.Org, w.Name).Set(1)
			} else {
				metrics.WorkspaceRunning.WithLabelValues(w.Org, w.Name).Set(0)
			}
		}
	}()
	return nil
}

func refreshMetrics() {
	l := log.WithField("func", "refreshMetrics")
	l.Debug("refreshing metrics")
	for {
		time.Sleep(10 * time.Second)
		err := getMetrics()
		if err != nil {
			l.WithError(err).Error("error getting metrics")
		}
	}
}

func Server(port int) error {
	l := log.WithFields(log.Fields{
		"pkg": "ws",
		"fn":  "Server",
	})
	l.Debug("start")
	if err := db.Init(); err != nil {
		l.Fatal(err)
	}
	db.DB.AutoMigrate(&Workspace{})
	metrics.Init()
	go refreshMetrics()
	r := mux.NewRouter()
	r.HandleFunc("/orgs", HandleListOrgs).Methods("GET")
	r.HandleFunc("/orgs/status-count", HandleAllStatusCount).Methods("GET")
	r.HandleFunc("/ws", HandleSaveWorkspace).Methods("PUT", "POST")
	r.HandleFunc("/ws/org/like", HandleListOrgWorkspacesLike).Methods("GET")
	r.HandleFunc("/ws/all", HandleListAllWorkspaces).Methods("GET")
	r.HandleFunc("/ws/all/like", HandleListAllWorkspacesLike).Methods("GET")
	r.HandleFunc("/ws/{org}/status-count", HandleOrgStatusCount).Methods("GET")
	r.HandleFunc("/ws/status/{status}", HandleListAllWorkspacesByStatus).Methods("GET")
	r.HandleFunc("/ws/org/{org}", HandleListOrgWorkspaces).Methods("GET")
	r.HandleFunc("/ws/{org}/{name}", HandleDeleteWorkspace).Methods("DELETE")
	r.HandleFunc("/ws/{org}/{name}", HandleGetWorkspace).Methods("GET")
	r.HandleFunc("/ws/{org}/status/{status}", HandleListOrgWorkspacesByStatus).Methods("GET")
	r.HandleFunc("/meta/statuses", HandleListValidStatuses).Methods("GET")
	r.Handle("/metrics", promhttp.Handler())
	l.WithField("port", port).Info("starting server")
	return http.ListenAndServe(fmt.Sprintf(":%d", port), r)
}
