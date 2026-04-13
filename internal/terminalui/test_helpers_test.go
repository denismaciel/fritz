package terminalui

import (
	"reflect"
	"testing"
	"unsafe"

	"fritz/internal/agent"
	"fritz/internal/config"
)

func setRuntimeModelID(t *testing.T, runtime *agent.Runtime, modelID string) {
	t.Helper()
	value := reflect.ValueOf(runtime).Elem().FieldByName("cfg")
	cfgPtr := reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
	cfgPtr.Set(reflect.ValueOf(config.Runtime{ModelID: modelID}))
}
