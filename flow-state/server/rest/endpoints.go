package rest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	flowEvent "github.com/project-flogo/flow/support/event"
	"github.com/project-flogo/services/flow-state/event"
	"github.com/project-flogo/services/flow-state/store/metadata"

	"github.com/julienschmidt/httprouter"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/flow/state"
	"github.com/project-flogo/services/flow-state/store"
)

const (
	Flogo_UserName       = "username"
	FLOGO_APPNAME        = "app"
	FLOGO_APPVERSION     = "version"
	FLOGO_HOSTNAME       = "host"
	FLOGO_FlowName       = "flow"
	Flow_Status          = "status"
	Flow_Failed_Mode     = "failed"
	Flow_Completed_Mode  = "completed"
	OFFSET               = "offset"
	LIMIT                = "limit"
	INTERVAL             = "interval"
	FLOW_INSTANCE_ID     = "flowinstanceid"
	ASYNC_CALLING_HEADER = "Async-Calling"
)

type ServiceEndpoints struct {
	logger        log.Logger
	stepStore     store.Store
	streamingStep bool
}

func AppendEndpoints(router *httprouter.Router, logger log.Logger, exposeRecorder bool, streamingStep bool) {

	sm := &ServiceEndpoints{
		logger:    logger,
		stepStore: store.RegistedStore(),
	}
	router.GET("/v1/health", sm.getHealthCheck)
	router.GET("/v1/instances", sm.getInstances)

	router.GET("/v1/instances/:flowId/details", sm.getInstance)
	router.GET("/v1/instances/:flowId/status", sm.getStatus)

	router.GET("/v1/instances/:flowId/steps", sm.getSteps)
	router.GET("/v1/instances/:flowId/steps/tasks", sm.getStepsAsTasks)
	router.GET("/v1/instances/:flowId/steps/status", sm.getStepsStatus)
	router.GET("/v1/instances/:flowId/step/:stepId/taskdata", sm.getStepdataForActivity)
	router.GET("/v1/flows", sm.getFlowNames)

	if streamingStep {
		router.GET("/v1/stream/steps", event.HandleStepEvent)
		event.StartStepListener()
	}

	router.GET("/v1/instances/:flowId/snapshot", sm.getSnapshot)
	router.GET("/v1/instances/:flowId/snapshot/:stepId", sm.getSnapshotAtStep)
	router.DELETE("/v1/instances/:flowId", sm.deleteInstance)
	router.DELETE("/v1/instances/:flowId/step/:stepId", sm.deleteSteps)
	router.GET("/v1/instances/:flowId/failedtask", sm.getFaildTaskStepId)

	if exposeRecorder {
		router.POST("/v1/instances/snapshot", sm.saveSnapshot)
		router.POST("/v1/instances/steps", sm.saveStep)
		router.POST("/v1/instances/start", sm.saveStart)
		router.POST("/v1/instances/end", sm.saveEnd)
	}
}

func (se *ServiceEndpoints) getHealthCheck(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	se.logger.Debugf("Endpoint[GET:/health] : Called")
	switch request.Method {
	case http.MethodGet:
		response.WriteHeader(http.StatusOK)
	default:
		response.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (se *ServiceEndpoints) getInstances(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	se.logger.Debugf("Endpoint[GET:/instances] : Called")

	userName := request.Header.Get(Flogo_UserName)
	if len(userName) <= 0 {
		http.Error(response, "unauthorized, please provide user information", http.StatusUnauthorized)
		return
	}

	appName := request.URL.Query().Get(FLOGO_APPNAME)
	if len(appName) <= 0 {
		http.Error(response, "Please provide app name", http.StatusBadRequest)
		return
	}

	appVersion := request.URL.Query().Get(FLOGO_APPVERSION)
	if len(appVersion) <= 0 {
		http.Error(response, "Please provide app version", http.StatusBadRequest)
		return
	}

	metadata := &metadata.Metadata{
		Username:   userName,
		AppName:    appName,
		AppVersion: appVersion,
		HostId:     request.URL.Query().Get(FLOGO_HOSTNAME),
		FlowName:   request.URL.Query().Get(FLOGO_FlowName),
	}

	offsetValue := request.URL.Query().Get(OFFSET)
	if len(offsetValue) > 0 {
		metadata.Offset = offsetValue
	}

	limitValue := request.URL.Query().Get(LIMIT)
	if len(limitValue) > 0 {
		metadata.Limit = limitValue
	}

	// var instances []*state.FlowInfo
	// var err error
	status := request.URL.Query().Get(Flow_Status)
	if len(status) > 0 {
		metadata.Status = status
	}

	flowinstanceid := request.URL.Query().Get(FLOW_INSTANCE_ID)
	if len(flowinstanceid) > 0 {
		metadata.FlowInstanceId = flowinstanceid
	}

	interval := request.URL.Query().Get(INTERVAL)
	if len(interval) > 0 {
		metadata.Interval = interval
	}
	/*if len(status) > 0 && mode == Flow_Failed_Mode {
		instances, err = se.stepStore.GetFailedFlows(metadata)
		if err != nil {
			http.Error(response, "Getting flow instance error:"+err.Error(), http.StatusInternalServerError)
			return
		}
	} else if len(mode) > 0 && mode == Flow_Completed_Mode {
		instances, err = se.stepStore.GetCompletedFlows(metadata)
		if err != nil {
			http.Error(response, "Getting flow instance error:"+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {*/
	instances, err := se.stepStore.GetFlowsWithRecordCount(metadata)
	if err != nil {
		http.Error(response, "Getting flow instance error:"+err.Error(), http.StatusInternalServerError)
		return
	}

	/*}*/

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if instances == nil {
		_, _ = response.Write([]byte("[]"))
		return
	}
	if err := json.NewEncoder(response).Encode(instances); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getInstance(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/details] : Called", flowId)

	userName := request.Header.Get(Flogo_UserName)
	if len(userName) <= 0 {
		http.Error(response, "unauthorized, please provide user information", http.StatusUnauthorized)
		return
	}

	metadata := &metadata.Metadata{
		Username:   userName,
		AppName:    request.URL.Query().Get(FLOGO_APPNAME),
		AppVersion: request.URL.Query().Get(FLOGO_APPVERSION),
		HostId:     request.URL.Query().Get(FLOGO_HOSTNAME),
		FlowName:   request.URL.Query().Get(FLOGO_FlowName),
	}

	instance, err := se.stepStore.GetFlow(flowId, metadata)
	if err != nil {
		http.Error(response, "get flow details error: ", http.StatusInternalServerError)
		return
	}
	if instance == nil {
		se.logger.Debugf("Getting instance from steps")
		instance, err = se.stepStore.GetFlow(flowId, metadata)
		if err != nil {
			http.Error(response, "get flow details error: ", http.StatusInternalServerError)
			return
		}
		if instance == nil {
			response.WriteHeader(http.StatusNotFound)
			return
		}
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(instance); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getStatus(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/status] : Called", flowId)
	status := se.stepStore.GetStatus(flowId)

	if status == -1 {
		se.logger.Debugf("Getting status from steps")
		status = se.stepStore.GetStatus(flowId)

		if status == -1 {
			response.WriteHeader(http.StatusNotFound)
			return
		}
	}

	statusObj := make(map[string]string, 1)
	statusObj["status"] = strconv.Itoa(status)

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(statusObj); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getSteps(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/steps] : Called", flowId)
	steps, err := se.stepStore.GetSteps(flowId)
	if err != nil {
		http.Error(response, "get steps error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	if steps == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(steps); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getStepsStatus(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/steps/status] : Called", flowId)
	steps, err := se.stepStore.GetStepsStatus(flowId)
	if err != nil {
		http.Error(response, "get steps error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	if steps == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(steps); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getStepsAsTasks(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/steps/tasks] : Called", flowId)
	tasks, err := se.stepStore.GetStepsAsTasks(flowId)
	if err != nil {
		http.Error(response, "get tasks error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(tasks); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) deleteSteps(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	stepId := params.ByName("stepId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/step/%s] : Called", flowId, stepId)
	err := se.stepStore.DeleteSteps(flowId, stepId)
	if err != nil {
		http.Error(response, "get error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (se *ServiceEndpoints) getStepdataForActivity(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	stepid := params.ByName("stepId")
	taskname := request.URL.Query().Get("taskName")
	se.logger.Debugf("Endpoint[GET:/instances/%s/step/%s/taskdata] : Called", flowId, stepid)
	stepdata, err := se.stepStore.GetStepdataForActivity(flowId, stepid, taskname)
	if err != nil {
		http.Error(response, "get tasks error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	if stepdata == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(stepdata); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getFlowNames(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	se.logger.Debugf("Endpoint[GET:/flows] : Called")

	userName := request.Header.Get(Flogo_UserName)
	if len(userName) <= 0 {
		http.Error(response, "unauthorized, please provide user information", http.StatusUnauthorized)
		return
	}

	appName := request.URL.Query().Get(FLOGO_APPNAME)
	if len(appName) <= 0 {
		http.Error(response, "Please provide app name", http.StatusBadRequest)
		return
	}

	appVersion := request.URL.Query().Get(FLOGO_APPVERSION)
	if len(appVersion) <= 0 {
		http.Error(response, "Please provide app version", http.StatusBadRequest)
		return
	}

	metadata := &metadata.Metadata{
		Username:   userName,
		AppName:    appName,
		AppVersion: appVersion,
		HostId:     request.URL.Query().Get(FLOGO_HOSTNAME),
	}
	flownames, err := se.stepStore.GetFlowNames(metadata)
	if err != nil {
		http.Error(response, "Getting flow instance error:"+err.Error(), http.StatusInternalServerError)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if flownames == nil || len(flownames) < 1 {
		_, _ = response.Write([]byte("[]"))
		return
	}
	if err := json.NewEncoder(response).Encode(flownames); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getSnapshot(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/snapshot] : Called", flowId)
	snapshot := se.stepStore.GetSnapshot(flowId)

	if snapshot == nil {
		se.logger.Debugf("Getting Snapshot from steps")
		steps, err := se.stepStore.GetSteps(flowId)
		if err != nil {
			http.Error(response, "get getSnapshot error:"+err.Error(), http.StatusInternalServerError)
			return
		}
		if steps == nil {
			response.WriteHeader(http.StatusNotFound)
			return
		}

		snapshot = state.StepsToSnapshot(flowId, steps)
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(snapshot); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getSnapshotAtStep(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	stepIdStr := params.ByName("stepId")

	se.logger.Debugf("Endpoint[GET:/instances/%s/snapshot/%s] : Called", flowId, stepIdStr)
	steps, err := se.stepStore.GetSteps(flowId)
	if err != nil {
		http.Error(response, "get getSnapshotAtStep error:"+err.Error(), http.StatusInternalServerError)
		return
	}
	if steps == nil {
		response.WriteHeader(http.StatusNotFound)
		return
	}

	stepId, err := strconv.Atoi(stepIdStr)
	if err != nil {
		se.error(response, http.StatusBadRequest, fmt.Errorf("invalid stepId: %s", stepIdStr))
		se.logger.Errorf("Endpoint[GET:/instances/%s/snapshot/%s] : Invalid StepId")
		return
	}

	if stepId >= len(steps) {
		se.error(response, http.StatusBadRequest, fmt.Errorf("invalid stepId: %d, only %d exists", stepId, len(steps)))
		se.logger.Errorf("Endpoint[GET:/instances/%s/snapshot/%s] : Step does not exists")
		return
	}

	snapshot := state.StepsToSnapshot(flowId, steps[:stepId+1])

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(snapshot); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) getFaildTaskStepId(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[GET:/instances/%s/failedtask] : Called", flowId)
	steps, err := se.stepStore.GetStepsStatus(flowId)
	if err != nil {
		http.Error(response, "get getSnapshotAtStep error:"+err.Error(), http.StatusInternalServerError)
		return
	}

	var stepID, taskName string
	for _, s := range steps {
		if s["status"] == flowEvent.FAILED {
			stepID = s["stepId"]
			taskName = s["taskName"]
		}
	}

	returnData := map[string]string{"flowInstanceId": flowId, "stepId": stepID, "taskName": taskName}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(response).Encode(returnData); err != nil {
		se.logger.Error(err.Error())
	}
}

func (se *ServiceEndpoints) deleteInstance(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	flowId := params.ByName("flowId")
	se.logger.Debugf("Endpoint[DEL:/instances/%s] : Called", flowId)

	//se.stepStore.Delete(flowId)
	se.stepStore.Delete(flowId)

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
}

func (se *ServiceEndpoints) saveStart(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	se.logger.Debugf("Endpoint[POST:/instances/start] : Called")
	asyncCalling := request.Header.Get(ASYNC_CALLING_HEADER) == "true"
	content, err := ioutil.ReadAll(request.Body)
	if err != nil {
		se.error(response, http.StatusBadRequest, fmt.Errorf("unable to read body"))
		se.logger.Error("Endpoint[POST:/instances/start] : %v", err)
		return
	}
	if asyncCalling {
		se.logger.Debug("Calling saveStart in Async way")
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		se.logger.Debug("Response sent with StatusAccepted.")

		go func() {
			step := &state.FlowState{}
			err = json.Unmarshal(content, step)
			if err != nil {
				se.logger.Debugf("Endpoint[POST:/instances/start] : Step content - %s ", string(content))
				se.logger.Errorf("Endpoint[POST:/instances/start] : Error unmarshalling step - %v", err)
				return
			}
			err = se.stepStore.RecordStart(step)
			if err != nil {
				se.logger.Errorf("Endpoint[POST:/instances/start] : Error saving step - %v", err)
				return
			}
		}()
	} else {
		step := &state.FlowState{}
		err = json.Unmarshal(content, step)
		if err != nil {
			se.error(response, http.StatusBadRequest, fmt.Errorf("unable to unmarshal step json"))
			se.logger.Debugf("Endpoint[POST:/instances/start] : Step content - %s ", string(content))
			se.logger.Errorf("Endpoint[POST:/instances/start] : Error unmarshalling step - %v", err)
			return
		}

		err = se.stepStore.RecordStart(step)
		if err != nil {
			se.error(response, http.StatusInternalServerError, fmt.Errorf("unable to save step"))
			se.logger.Errorf("Endpoint[POST:/instances/start] : Error saving step - %v", err)
			return
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
	}

}

func (se *ServiceEndpoints) saveStep(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	se.logger.Debugf("Endpoint[POST:/instances/steps] : Called")
	asyncCalling := request.Header.Get(ASYNC_CALLING_HEADER) == "true"
	content, err := ioutil.ReadAll(request.Body)
	if err != nil {
		se.error(response, http.StatusBadRequest, fmt.Errorf("unable to read body"))
		se.logger.Error("Endpoint[POST:/instances/steps] : %v", err)
		return
	}
	if asyncCalling {
		se.logger.Debug("Calling saveStep in Async way")
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		se.logger.Debug("Response sent with StatusAccepted.")
		go func() {
			step := &state.Step{}
			err = json.Unmarshal(content, step)
			if err != nil {
				se.logger.Debugf("Endpoint[POST:/instances/steps] : Step content - %s ", string(content))
				se.logger.Errorf("Endpoint[POST:/instances/steps] : Error unmarshalling step - %v", err)
				return
			}
			err = se.stepStore.SaveStep(step)
			if err != nil {
				se.logger.Errorf("Endpoint[POST:/instances/steps] : Error saving step - %v", err)
				return
			}
		}()
	} else {
		step := &state.Step{}
		err = json.Unmarshal(content, step)
		if err != nil {
			se.error(response, http.StatusBadRequest, fmt.Errorf("unable to unmarshal step json"))
			se.logger.Debugf("Endpoint[POST:/instances/steps] : Step content - %s ", string(content))
			se.logger.Errorf("Endpoint[POST:/instances/steps] : Error unmarshalling step - %v", err)
			return
		}
		err = se.stepStore.SaveStep(step)
		if err != nil {
			se.error(response, http.StatusInternalServerError, fmt.Errorf("unable to save step"))
			se.logger.Errorf("Endpoint[POST:/instances/steps] : Error saving step - %v", err)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
	}
}

func (se *ServiceEndpoints) saveSnapshot(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	se.logger.Debugf("Endpoint[POST:/instances/snapshot] : Called")
	asyncCalling := request.Header.Get(ASYNC_CALLING_HEADER) == "true"
	content, err := ioutil.ReadAll(request.Body)
	if err != nil {
		se.error(response, http.StatusBadRequest, fmt.Errorf("unable to read body"))
		se.logger.Error("Endpoint[POST:/instances/snapshot] : %v", err)
		return
	}

	if asyncCalling {
		se.logger.Debug("Calling saveSnapshot in Async way")
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		se.logger.Debug("Response sent with StatusAccepted.")

		go func() {
			snapshot := &state.Snapshot{SnapshotBase: &state.SnapshotBase{}}
			err = json.Unmarshal(content, snapshot)
			if err != nil {
				se.logger.Debugf("Endpoint[POST:/instances/snapshot] : Snapshot content - %s ", string(content))
				se.logger.Errorf("Endpoint[POST:/instances/snapshot] : Error unmarshalling snapshot - %v", err)
				return
			}
			err = se.stepStore.SaveSnapshot(snapshot)
			if err != nil {
				se.logger.Errorf("Endpoint[POST:/instances/snapshot] : Error saving snapshot - %v", err)
				return
			}
		}()
	} else {
		snapshot := &state.Snapshot{SnapshotBase: &state.SnapshotBase{}}
		err = json.Unmarshal(content, snapshot)
		if err != nil {
			se.error(response, http.StatusBadRequest, fmt.Errorf("unable to unmarshal snapshot json"))
			se.logger.Debugf("Endpoint[POST:/instances/snapshot] : Snapshot content - %s ", string(content))
			se.logger.Errorf("Endpoint[POST:/instances/snapshot] : Error unmarshalling snapshot - %v", err)
			return
		}
		err = se.stepStore.SaveSnapshot(snapshot)
		if err != nil {
			se.error(response, http.StatusInternalServerError, fmt.Errorf("unable to save snapshot"))
			se.logger.Errorf("Endpoint[POST:/instances/snapshot] : Error saving snapshot - %v", err)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
	}
}

func (se *ServiceEndpoints) saveEnd(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	se.logger.Debugf("Endpoint[POST:/instances/end] : Called")
	asyncCalling := request.Header.Get(ASYNC_CALLING_HEADER) == "true"
	content, err := ioutil.ReadAll(request.Body)
	if err != nil {
		se.error(response, http.StatusBadRequest, fmt.Errorf("unable to read body"))
		se.logger.Error("Endpoint[POST:/instances/end] : %v", err)
		return
	}

	if asyncCalling {
		se.logger.Debug("Calling saveEnd in Async way")
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		se.logger.Debug("Response sent with StatusAccepted.")

		go func() {
			step := &state.FlowState{}
			err = json.Unmarshal(content, step)
			if err != nil {
				se.logger.Debugf("Endpoint[POST:/instances/end] : Step content - %s ", string(content))
				se.logger.Errorf("Endpoint[POST:/instances/end] : Error unmarshalling step - %v", err)
				return
			}
			err = se.stepStore.RecordEnd(step)
			if err != nil {
				se.logger.Errorf("Endpoint[POST:/instances/end] : Error saving step - %v", err)
				return
			}
		}()
	} else {
		step := &state.FlowState{}
		err = json.Unmarshal(content, step)
		if err != nil {
			se.error(response, http.StatusBadRequest, fmt.Errorf("unable to unmarshal step json"))
			se.logger.Debugf("Endpoint[POST:/instances/end] : Step content - %s ", string(content))
			se.logger.Errorf("Endpoint[POST:/instances/end] : Error unmarshalling step - %v", err)
			return
		}
		err = se.stepStore.RecordEnd(step)
		if err != nil {
			se.error(response, http.StatusInternalServerError, fmt.Errorf("unable to save step"))
			se.logger.Errorf("Endpoint[POST:/instances/end] : Error saving step - %v", err)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
	}
}

type StateError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (se *ServiceEndpoints) error(response http.ResponseWriter, code int, err error) {
	flowError := &StateError{
		Code:    code,
		Message: err.Error(),
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(code)
	if err := json.NewEncoder(response).Encode(flowError); err != nil {
		se.logger.Errorf("unable to encode err to json: %v", err)
	}
}
