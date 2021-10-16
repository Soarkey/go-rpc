package server

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// methodType 远程调用的函数形式
// func (t *T) MethodName(argType T1, replyType *T2) error
type methodType struct {
	method    reflect.Method // 方法本身
	ArgType   reflect.Type   // 第一个参数类型
	ReplyType reflect.Type   // 第二个参数类型
	numCalls  uint64         // 统计调用次数
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

func (m *methodType) newArgv() (argv reflect.Value) {
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
		return
	}
	argv = reflect.New(m.ArgType).Elem()
	return
}

func (m *methodType) newReplyv() reflect.Value {
	// reply 必须为指针类型
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

// service 服务结构体
type service struct {
	name   string                 // 映射的结构体类型
	typ    reflect.Type           // 结构体类型
	rcvr   reflect.Value          // 结构体实例本身, 需要作为第0个参数
	method map[string]*methodType // 存储映射的结构体的所有符合条件的方法
}

// registerMethods 注册所有方法
func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		// 过滤符合条件的方法:
		// 两个导出或内置类型的入参 (反射时为 3 个, 第 0 个是自身, 类似于 python 的 self, java 中的 this)
		// 返回值有且只有 1 个, 类型为 error
		if mType.NumIn() != 3 || mType.NumOut() != 1 || mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: 注册 %s.%s", s.name, method.Name)
	}
}

// isExportedOrBuiltinType 判断是否为可导出方法或内置方法
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// call 通过反射值调用方法
func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}

// newService 将入参结构体 rcvr 映射为服务
func newService(rcvr interface{}) *service {
	s := new(service)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = reflect.TypeOf(rcvr)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: 方法 %s 不可导出, 不是一个有效的服务", s.name)
	}
	s.registerMethods()
	return s
}
