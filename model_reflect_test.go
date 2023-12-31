package model_reflect_test

import (
	"testing"
	"time"

	"github.com/go-modern/model_reflect"
)

type BigInt int

type PrivStruct struct {
	BigInt
	Lolipop int64
	Data    float32
	T       time.Time `cbor:"time"`
}

type TestStruct struct {
	Lolipop float32
	PrivStruct
	Stuff int `json:"stuff,omitempty"`
	Data  int `json:",omitempty"`
	//nolint:unused
	thing string
}

type testStruct2 struct {
	//nolint:unused
	string
	Lolipop string
	Func    func()
	PrivStruct
	*TestStruct
	// Data   string
	Wow  [2]struct{ Test float64 }
	Derp string `reflect:"-"`
	//nolint:unused
	thing2 string
	Ok     *****map[****struct{ Dude byte }]****TestStruct
}

func TestModelReflect(t *testing.T) {
	model, err := model_reflect.New(nil)
	id := model.Hash()
	t.Logf("NillModelReflect: (%d) %s [%v]", id, model, err)
	model, err = model_reflect.New((*testStruct2)(nil))
	id = model.Hash()
	// testStruct := &testStruct2{}
	t.Logf("TestModelReflect: (%d) %s [%v]", id, model, err)
	if id != 4637779922895058862 {
		t.Error("test")
	}
}

type testA struct {
	A int
	// X *testB
	// B testB
	*testB
	X *testB
}

type testB struct {
	B int
	*testA
}

func TestModelReflectRecursive(t *testing.T) {
	model, err := model_reflect.New((*testA)(nil))
	id := model.Hash()
	t.Logf("TestModelReflectRecursive: (%d) %s [%v]", id, model, err)
	if id != 879603111333480357 {
		t.Error("test")
	}
}
