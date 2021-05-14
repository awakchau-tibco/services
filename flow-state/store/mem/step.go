package mem

import (
	"github.com/project-flogo/flow/state"
	"github.com/project-flogo/services/flow-state/event"
	"github.com/project-flogo/services/flow-state/store/metadata"
	"sync"
)

//func init() {
//	store.SetStepStore(&StepStore{stepContainers: make(map[string]*stepContainer)})
//}

func NewStore() *StepStore {
	return &StepStore{stepContainers: make(map[string]*stepContainer)}
}

type StepStore struct {
	sync.RWMutex
	userId         string
	appId          string
	stepContainers map[string]*stepContainer
	snapshots      sync.Map
}

func (s *StepStore) GetStatus(flowId string) int {

	s.RLock()
	sc, ok := s.stepContainers[flowId]
	s.RUnlock()

	if ok {
		return sc.Status()
	}

	return -1
}

func (s *StepStore) GetFlow(flowId string) *state.FlowInfo {

	s.RLock()
	sc, ok := s.stepContainers[flowId]
	s.RUnlock()

	if ok {
		return &state.FlowInfo{Id: flowId, Status: sc.Status(), FlowURI: sc.flowURI}
	}

	return nil
}

func (s *StepStore) GetFlows(metadata *metadata.Metadata) ([]*state.FlowInfo, error) {

	var infos []*state.FlowInfo

	s.RLock()
	for id, value := range s.stepContainers {
		infos = append(infos, &state.FlowInfo{Id: id, FlowURI: value.flowURI, Status: value.Status()})
	}
	s.RUnlock()

	return infos, nil
}

func (s *StepStore) GetFailedFlows(metadata *metadata.Metadata) ([]*state.FlowInfo, error) {

	var infos []*state.FlowInfo

	s.RLock()
	for id, value := range s.stepContainers {
		if value.Status() == 500 {
			infos = append(infos, &state.FlowInfo{Id: id, FlowURI: value.flowURI, Status: value.Status()})
		}
	}
	s.RUnlock()

	return infos, nil
}

func (s *StepStore) SaveStep(step *state.Step) error {
	event.PostStepEvent(step)
	s.RLock()
	sc, ok := s.stepContainers[step.FlowId]
	s.RUnlock()

	if !ok {
		s.Lock()
		sc, ok = s.stepContainers[step.FlowId]
		if !ok {
			sc = &stepContainer{}
		}
		s.stepContainers[step.FlowId] = sc
		s.Unlock()
	}

	sc.AddStep(step)

	return nil
}

func (s *StepStore) GetSteps(flowId string) []*state.Step {
	s.RLock()
	sc, ok := s.stepContainers[flowId]
	s.RUnlock()
	if ok {
		return sc.Steps()
	}

	return nil
}

func (s *StepStore) Delete(flowId string) {
	s.Lock()
	delete(s.stepContainers, flowId)
	s.Unlock()
}

type stepContainer struct {
	sync.RWMutex
	status  int
	flowURI string
	steps   []*state.Step
}

func (sc *stepContainer) Status() int {
	sc.RLock()
	status := sc.status
	sc.RUnlock()

	return status
}

func (sc *stepContainer) AddStep(step *state.Step) {
	sc.Lock()

	if len(step.FlowChanges) > 0 {
		if step.FlowChanges[0] != nil && step.FlowChanges[0].SubflowId == 0 {
			if status := step.FlowChanges[0].Status; status != -1 {
				sc.status = status
			}
			if uri := step.FlowChanges[0].FlowURI; uri != "" {
				sc.flowURI = uri
			}
		}
	}

	sc.steps = append(sc.steps, step)
	sc.Unlock()
}

func (sc *stepContainer) Steps() []*state.Step {
	sc.RLock()
	steps := sc.steps
	sc.RUnlock()
	return steps
}

func (s *StepStore) SaveSnapshot(snapshot *state.Snapshot) error {
	//replaces existing snapshot
	s.snapshots.Store(snapshot.Id, snapshot)
	return nil
}

func (s *StepStore) GetSnapshot(flowId string) *state.Snapshot {
	if snapshot, ok := s.snapshots.Load(flowId); ok {
		return snapshot.(*state.Snapshot)
	}
	return nil
}

func (s *StepStore) RecordStart(flowState *state.FlowState) error {
	return nil
}

func (s *StepStore) RecordEnd(flowState *state.FlowState) error {
	return nil
}
