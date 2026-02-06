// Package ffi provides C-compatible bindings for aide.
// These can be called from TypeScript via node-ffi or similar.
//
// Build as shared library:
//
//	go build -buildmode=c-shared -o libaide_memory.so ./ffi
package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"encoding/json"
	"sync"
	"time"
	"unsafe"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

var (
	globalStore *store.BoltStore
	storeMutex  sync.Mutex
)

//export AideMemoryInit
func AideMemoryInit(dbPath *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore != nil {
		globalStore.Close()
	}

	path := C.GoString(dbPath)
	s, err := store.NewBoltStore(path)
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	globalStore = s
	return C.CString(`{"success":true}`)
}

//export AideMemoryClose
func AideMemoryClose() {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore != nil {
		globalStore.Close()
		globalStore = nil
	}
}

//export AideMemoryAdd
func AideMemoryAdd(category, content, tags, plan *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	m := &memory.Memory{
		ID:        generateID(),
		Category:  memory.Category(C.GoString(category)),
		Content:   C.GoString(content),
		Plan:      C.GoString(plan),
		Priority:  1.0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Parse tags if provided
	tagsStr := C.GoString(tags)
	if tagsStr != "" {
		json.Unmarshal([]byte(tagsStr), &m.Tags)
	}

	if err := globalStore.AddMemory(m); err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(m)
	return C.CString(string(result))
}

//export AideMemorySearch
func AideMemorySearch(query *C.char, limit C.int) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	memories, err := globalStore.SearchMemories(C.GoString(query), int(limit))
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(memories)
	return C.CString(string(result))
}

//export AideMemoryList
func AideMemoryList(category *C.char, limit C.int) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	opts := memory.SearchOptions{
		Category: memory.Category(C.GoString(category)),
		Limit:    int(limit),
	}

	memories, err := globalStore.ListMemories(opts)
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(memories)
	return C.CString(string(result))
}

//export AideMemoryTaskCreate
func AideMemoryTaskCreate(title, description *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	t := &memory.Task{
		ID:          generateID(),
		Title:       C.GoString(title),
		Description: C.GoString(description),
		Status:      memory.TaskStatusPending,
		CreatedAt:   time.Now(),
	}

	if err := globalStore.AddTask(t); err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(t)
	return C.CString(string(result))
}

//export AideMemoryTaskClaim
func AideMemoryTaskClaim(taskID, agentID *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	task, err := globalStore.ClaimTask(C.GoString(taskID), C.GoString(agentID))
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(task)
	return C.CString(string(result))
}

//export AideMemoryTaskComplete
func AideMemoryTaskComplete(taskID, resultStr *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	if err := globalStore.CompleteTask(C.GoString(taskID), C.GoString(resultStr)); err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	return C.CString(`{"success":true}`)
}

//export AideMemoryTaskList
func AideMemoryTaskList(status *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	tasks, err := globalStore.ListTasks(memory.TaskStatus(C.GoString(status)))
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(tasks)
	return C.CString(string(result))
}

//export AideMemoryDecisionSet
func AideMemoryDecisionSet(topic, decision, rationale *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	d := &memory.Decision{
		Topic:     C.GoString(topic),
		Decision:  C.GoString(decision),
		Rationale: C.GoString(rationale),
		CreatedAt: time.Now(),
	}

	if err := globalStore.SetDecision(d); err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(d)
	return C.CString(string(result))
}

//export AideMemoryDecisionGet
func AideMemoryDecisionGet(topic *C.char) *C.char {
	storeMutex.Lock()
	defer storeMutex.Unlock()

	if globalStore == nil {
		return C.CString(`{"error":"store not initialized"}`)
	}

	d, err := globalStore.GetDecision(C.GoString(topic))
	if err != nil {
		return C.CString(`{"error":"` + err.Error() + `"}`)
	}

	result, _ := json.Marshal(d)
	return C.CString(string(result))
}

//export AideMemoryFree
func AideMemoryFree(ptr *C.char) {
	C.free(unsafe.Pointer(ptr))
}

func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}

func main() {}
