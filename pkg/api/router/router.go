package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/rootless-containers/rootlesskit/v2/pkg/api"
	"github.com/rootless-containers/rootlesskit/v2/pkg/port"
	"github.com/rootless-containers/rootlesskit/v2/pkg/httputil"
	"github.com/rootless-containers/rootlesskit/v2/pkg/version"
)

// NetworkDriver is implemented by network.ParentDriver
type NetworkDriver interface {
	Info(context.Context) (*api.NetworkDriverInfo, error)
}

// PortDriver is implemented by port.ParentDriver
type PortDriver interface {
	Info(context.Context) (*api.PortDriverInfo, error)
	port.Manager
}

type Backend struct {
	StateDir string
	ChildPID int
	// NetworkDriver can be nil
	NetworkDriver NetworkDriver
	// PortDriver MUST be thread-safe.
	// PortDriver can be nil
	PortDriver PortDriver
}

func (b *Backend) onPortDriverNil(w http.ResponseWriter, r *http.Request) {
	httputil.WriteError(w, r, errors.New("no PortDriver is available"), http.StatusBadRequest)
}

// GetPorts is handler for GET /v{N}/ports
func (b *Backend) GetPorts(w http.ResponseWriter, r *http.Request) {
	if b.PortDriver == nil {
		b.onPortDriverNil(w, r)
		return
	}
	ports, err := b.PortDriver.ListPorts(context.TODO())
	if err != nil {
		httputil.WriteError(w, r, err, http.StatusInternalServerError)
		return
	}
	m, err := json.Marshal(ports)
	if err != nil {
		httputil.WriteError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

// PostPort is the handler for POST /v{N}/ports
func (b *Backend) PostPort(w http.ResponseWriter, r *http.Request) {
	if b.PortDriver == nil {
		b.onPortDriverNil(w, r)
		return
	}
	decoder := json.NewDecoder(r.Body)
	var portSpec port.Spec
	if err := decoder.Decode(&portSpec); err != nil {
		httputil.WriteError(w, r, err, http.StatusBadRequest)
		return
	}
	portStatus, err := b.PortDriver.AddPort(context.TODO(), portSpec)
	if err != nil {
		httputil.WriteError(w, r, err, http.StatusBadRequest)
		return
	}
	m, err := json.Marshal(portStatus)
	if err != nil {
		httputil.WriteError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(m)
}

// DeletePort is the handler for POST /v{N}/ports/{id}
func (b *Backend) DeletePort(w http.ResponseWriter, r *http.Request) {
	if b.PortDriver == nil {
		b.onPortDriverNil(w, r)
		return
	}
	idStr, ok := mux.Vars(r)["id"]
	if !ok {
		httputil.WriteError(w, r, errors.New("id not specified"), http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httputil.WriteError(w, r, fmt.Errorf("bad id %s: %w", idStr, err), http.StatusBadRequest)
		return
	}
	if err := b.PortDriver.RemovePort(context.TODO(), id); err != nil {
		httputil.WriteError(w, r, err, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (b *Backend) GetInfo(w http.ResponseWriter, r *http.Request) {
	info := &api.Info{
		APIVersion: api.Version,
		Version:    version.Version,
		StateDir:   b.StateDir,
		ChildPID:   b.ChildPID,
	}
	if b.NetworkDriver != nil {
		ndInfo, err := b.NetworkDriver.Info(context.Background())
		if err != nil {
			httputil.WriteError(w, r, err, http.StatusInternalServerError)
			return
		}
		info.NetworkDriver = ndInfo
	}
	if b.PortDriver != nil {
		pdInfo, err := b.PortDriver.Info(context.Background())
		if err != nil {
			httputil.WriteError(w, r, err, http.StatusInternalServerError)
			return
		}
		info.PortDriver = pdInfo
	}
	m, err := json.Marshal(info)
	if err != nil {
		httputil.WriteError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(m)
}

func AddRoutes(r *mux.Router, b *Backend) {
	v1 := r.PathPrefix("/v1").Subrouter()
	v1.Path("/ports").Methods("GET").HandlerFunc(b.GetPorts)
	v1.Path("/ports").Methods("POST").HandlerFunc(b.PostPort)
	v1.Path("/ports/{id}").Methods("DELETE").HandlerFunc(b.DeletePort)
	v1.Path("/info").Methods("GET").HandlerFunc(b.GetInfo)
}
