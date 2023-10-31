package main

import (
	"encoding/json"
	"fmt"
	ioutil "io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/gorilla/mux"

	"github.com/openshift/addon-operator/internal/ocm/ocmtest"
)

func main() {
	r := mux.NewRouter()
	addonStatusStore := NewAddonStatusStore()
	r.HandleFunc("/healthz", Health)
	r.HandleFunc("/readyz", Health)
	r.Handle(
		"/api/clusters_mgmt/v1/clusters",
		NewClustersEndpoint(),
	)
	r.Handle(
		"/api/clusters_mgmt/v1/clusters/{cluster_id}/addon_upgrade_policies/{upgrade_policy_id}/state",
		NewUpgradePolicyStateEndpoint(),
	)
	r.Handle(
		"/api/addons_mgmt/v1/clusters/{cluster_id}/status/{addon_id}",
		NewAddonStatusEndpoint(addonStatusStore),
	)
	r.Handle(
		"/api/addons_mgmt/v1/clusters/{cluster_id}/status",
		NewAddonStatusCreateEndpoint(addonStatusStore),
	)

	addr := ":8080"
	log.Printf("listening on %s\n", addr)

	//nolint: gosec
	if err := http.ListenAndServe(addr, r); err != nil {
		panic(err)
	}
}

func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type ClustersEndpoint struct {
	data    map[ClustersKey]string
	dataMux sync.RWMutex
}

func NewClustersEndpoint() *ClustersEndpoint {
	return &ClustersEndpoint{
		data: map[ClustersKey]string{},
	}
}

type ClustersKey struct {
	ExternalId string
}

func (cs *ClustersEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	re := regexp.MustCompile(`'.*'`)

	switch r.Method {

	case http.MethodGet:
		cs.dataMux.RLock()
		defer cs.dataMux.RUnlock()

		w.WriteHeader(http.StatusOK)

		//for the mock server, we don't care about the expression itself,
		//we just want the cluster external id out of it
		//e.g.: external_id = 'a440b136-b2d6-406b-a884-fca2d62cd170'
		//get the id, with quotes
		search := r.URL.Query().Get("search")
		idFromSearch := re.FindStringSubmatch(search)

		//safeguard, when there's no cluster id in the search
		//string, we return an empty list of clusters
		if len(idFromSearch) == 0 {
			fmt.Fprintf(w, `{"items": []}`)
			return
		}

		//remove the quotes
		clusterExternalId := strings.Trim(idFromSearch[0], "'")

		//return always the same cluster id, regardless the external id
		//provided
		fmt.Fprintf(w,
			`{"items": [{"kind": "Cluster","id": "%s","name": "%s", "external_id": "%s"}]}`,
			ocmtest.MockClusterId,
			ocmtest.MockClusterName,
			clusterExternalId)

		log.Printf("%s %s:\n", r.URL.String(), r.Method)

	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
}

type UpgradePolicyStateEndpoint struct {
	data    map[UpgradePolicyStateKey]string
	dataMux sync.RWMutex
}

func NewUpgradePolicyStateEndpoint() *UpgradePolicyStateEndpoint {
	return &UpgradePolicyStateEndpoint{
		data: map[UpgradePolicyStateKey]string{},
	}
}

type addonStatusStore struct {
	data    map[addonStatusKey]addonStatus
	dataMux sync.RWMutex
}

type addonStatusKey struct {
	clusterID string
	addonID   string
}

type addonStatus struct {
	AddonID       string `json:"addon_id"`
	CorrelationID string `json:"correlation_id"`
	AddonVersion  string `json:"version"`
	// We dont care about this unmarshalling this field.
	StatusConditions []interface{} `json:"status_conditions"`
}

func NewAddonStatusStore() *addonStatusStore {
	return &addonStatusStore{
		data: map[addonStatusKey]addonStatus{},
	}
}

type AddonStatusCreateEndpoint struct {
	store *addonStatusStore
}

func NewAddonStatusCreateEndpoint(store *addonStatusStore) *AddonStatusCreateEndpoint {
	return &AddonStatusCreateEndpoint{
		store: store,
	}
}

func (a *AddonStatusCreateEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.store.dataMux.Lock()
		defer a.store.dataMux.Unlock()
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("reading request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		// unmarshal payload.
		status := addonStatus{}
		err = json.Unmarshal(payload, &status)
		if err != nil {
			log.Printf("unmarshalling request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		addonID := status.AddonID
		if len(addonID) == 0 {
			log.Printf("Missing addonID in addon status create request.")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, `{"code":"error","reason":"addonID missing"}`)
			return
		}
		vars := mux.Vars(r)
		a.store.data[addonStatusKey{
			addonID:   addonID,
			clusterID: vars["cluster_id"],
		}] = status
		log.Printf("%s %s:\n", r.URL.String(), r.Method)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, string(payload))
	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
}

type AddonStatusEndpoint struct {
	store *addonStatusStore
}

func NewAddonStatusEndpoint(store *addonStatusStore) *AddonStatusEndpoint {
	return &AddonStatusEndpoint{
		store: store,
	}
}

func (ase *AddonStatusEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ase.store.dataMux.RLock()
		defer ase.store.dataMux.RUnlock()
		vars := mux.Vars(r)
		data, ok := ase.store.data[addonStatusKey{
			clusterID: vars["cluster_id"],
			addonID:   vars["addon_id"],
		}]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, `{"code":"not found","reason":"addon status not found"}`)
			return
		}

		respBytes, err := marshalAddonStatus(data)
		if err != nil {
			log.Printf("marshaling response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(respBytes))
		log.Printf("%s %s:\n", r.URL.String(), r.Method)

	case http.MethodPost:
		ase.store.dataMux.Lock()
		defer ase.store.dataMux.Unlock()
		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("reading request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		// unmarshal payload.
		status, err := unmarshalPayloadToAddonStatus(payload)
		if err != nil {
			log.Printf("unmarshalling request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		vars := mux.Vars(r)

		ase.store.data[addonStatusKey{
			clusterID: vars["cluster_id"],
			addonID:   status.AddonID,
		}] = status
		respBytes, err := marshalAddonStatus(status)
		if err != nil {
			log.Printf("marshaling response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(respBytes))
		log.Printf("%s %s:\n", r.URL.String(), r.Method)

	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
}

type UpgradePolicyStateKey struct {
	ClusterID, UpgradePolicyID string
}

func (ups *UpgradePolicyStateEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		ups.dataMux.Lock()
		defer ups.dataMux.Unlock()

		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("reading request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{}`)
			return
		}

		vars := mux.Vars(r)
		ups.data[UpgradePolicyStateKey{
			ClusterID:       vars["cluster_id"],
			UpgradePolicyID: vars["upgrade_policy_id"],
		}] = string(payload)

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{}`)
		log.Printf("%s %s:\n%s\n", r.URL.String(), r.Method, payload)

	case http.MethodGet:
		ups.dataMux.RLock()
		defer ups.dataMux.RUnlock()

		vars := mux.Vars(r)
		data, ok := ups.data[UpgradePolicyStateKey{
			ClusterID:       vars["cluster_id"],
			UpgradePolicyID: vars["upgrade_policy_id"],
		}]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, `{"code":"not found","reason":"upgrade policy not found"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, data)
		log.Printf("%s %s:\n", r.URL.String(), r.Method)

	default:
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
}

func marshalAddonStatus(status addonStatus) ([]byte, error) {
	bytes, err := json.Marshal(status)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func unmarshalPayloadToAddonStatus(data []byte) (addonStatus, error) {
	status := addonStatus{}
	if err := json.Unmarshal(data, &status); err != nil {
		return addonStatus{}, err
	}
	return status, nil
}
