package callback

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"unsafe"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"

	"qshqn/core/db"
	"qshqn/core/qsh"
	"qshqn/core/typex"
)

const (
	InlineVersion1                         byte = 1
	InlineVersionSize                           = 1
	InlineCommandIDSize                         = 2
	InlineHeaderSize                            = InlineVersionSize + InlineCommandIDSize
	MaxInlineKeyboardCallbackDataBytes          = 64
	MaxInlineKeyboardCallbackDataArgsBytes      = MaxInlineKeyboardCallbackDataBytes - InlineHeaderSize
)

var (
	inlineKeyboardRegistry = [CmdIDLast]registeredCallback{}
	callbackCtxPool        = typex.NewPool(func() *Context {
		return &Context{}
	})
)

type Context struct {
	Ctx       context.Context
	Api       *tg.Client
	Sender    *message.Sender
	From      typex.From
	DBUser    *db.User
	Query     *tg.UpdateBotCallbackQuery
	CmdID     CmdID
	RawParams any
}

type CmdID uint16

type HandlerFunc = func(ctx *Context) error

type CallbackParams interface {
	Size() int
	Encode(dst []byte) int
	Decode(src []byte) error
}

type registeredCallback interface {
	size() int
	handler() HandlerFunc
	encode(self any, dst []byte) int
	decode(raw []byte) (any, error)
}

type TypedContext[T any] struct {
	*Context
	Params T
}

type TypedHandler[T any] func(ctx *TypedContext[T]) error

type callbackEntry[T any] struct {
	paramsSize int
	h          HandlerFunc
	make       func() T
	enc        func(self T, dst []byte) int
	dec        func(self T, src []byte) error
}

func (e *callbackEntry[T]) size() int            { return e.paramsSize }
func (e *callbackEntry[T]) handler() HandlerFunc { return e.h }

func (e *callbackEntry[T]) encode(self any, dst []byte) int {
	return e.enc(self.(T), dst)
}

func (e *callbackEntry[T]) decode(raw []byte) (any, error) {
	val := e.make()
	if err := e.dec(val, raw); err != nil {
		return nil, err
	}
	return val, nil
}

func GetContext(ctx context.Context, api *tg.Client, sender *message.Sender, from typex.From, dbUser *db.User, query *tg.UpdateBotCallbackQuery, cmdID CmdID, params any) *Context {
	c := callbackCtxPool.Get()
	c.Ctx = ctx
	c.Api = api
	c.Sender = sender
	c.From = from
	c.DBUser = dbUser
	c.Query = query
	c.CmdID = cmdID
	c.RawParams = params
	return c
}

func (c *Context) Put() {
	*c = Context{}
	callbackCtxPool.Put(c)
}

func Register[T any](
	cmdID CmdID,
	factory func() T,
	encode func(self T, dst []byte) int,
	decode func(self T, src []byte) error,
	handler TypedHandler[T],
) {
	if cmdID == CmdIDUnknown || cmdID >= CmdIDLast {
		panic(fmt.Sprintf("invalid cmdID[%d]", cmdID))
	}
	if inlineKeyboardRegistry[cmdID] != nil {
		panic(fmt.Sprintf("cmdID[%d] already registered", cmdID))
	}
	if factory == nil {
		panic(fmt.Sprintf("cmdID[%d] tried to register a nil factory", cmdID))
	}

	dummy := factory()
	if any(dummy) == nil {
		panic(fmt.Sprintf("cmdID[%d] factory returned a nil instance", cmdID))
	}

	size, err := calcTypeSize(reflect.TypeOf(dummy))
	if err != nil {
		panic(fmt.Errorf("cmdID[%d] calcCallbackParamsSize error: %w", cmdID, err))
	}
	if size > MaxInlineKeyboardCallbackDataArgsBytes {
		panic(fmt.Sprintf("cmdID[%d] payload size [%d] exceeds max [%d]", cmdID, size, MaxInlineKeyboardCallbackDataArgsBytes))
	}

	inlineKeyboardRegistry[cmdID] = &callbackEntry[T]{
		paramsSize: size,
		h: func(ctx *Context) error {
			params := ctx.RawParams.(T)
			return handler(&TypedContext[T]{Context: ctx, Params: params})
		},
		make: factory,
		enc:  encode,
		dec:  decode,
	}
}

func Encode(cmdID CmdID, params any) ([]byte, error) {
	if cmdID >= CmdIDLast {
		return nil, fmt.Errorf("cmdID[%d] is out of bounds", cmdID)
	}
	entry := inlineKeyboardRegistry[cmdID]
	if entry == nil {
		return nil, fmt.Errorf("cmdID[%d] not registered", cmdID)
	}
	paramsSize := entry.size()
	size := InlineHeaderSize
	if params != nil {
		size += paramsSize
		if size > MaxInlineKeyboardCallbackDataBytes {
			return nil, fmt.Errorf("payload exceeds [%d] bytes", MaxInlineKeyboardCallbackDataBytes)
		}
	}
	buf := make([]byte, size)
	buf[0] = InlineVersion1
	binary.BigEndian.PutUint16(buf[1:3], uint16(cmdID))

	if params != nil {
		if written := entry.encode(params, buf[InlineHeaderSize:]); written != paramsSize {
			return nil, fmt.Errorf("encode wrote [%d] bytes, expected [%d]", written, paramsSize)
		}
	}
	return buf, nil
}

func Decode[T any](data []byte) (CmdID, T, error) {
	var zero T
	dataLength := len(data)
	if dataLength < InlineHeaderSize {
		return CmdIDUnknown, zero, fmt.Errorf("callback data too short. header must be at least [%d] bytes, got [%d]", InlineHeaderSize, dataLength)
	}
	if data[0] != InlineVersion1 {
		return CmdIDUnknown, zero, fmt.Errorf("unsupported callback version: [%d]", data[0])
	}
	cmdID := CmdID(binary.BigEndian.Uint16(data[1:3]))
	if cmdID >= CmdIDLast {
		return cmdID, zero, fmt.Errorf("cmdID[%d] is out of bounds", cmdID)
	}
	entry := inlineKeyboardRegistry[cmdID]
	if entry == nil {
		return cmdID, zero, fmt.Errorf("cmdID[%d] not registered", cmdID)
	}
	actualArgSize := dataLength - InlineHeaderSize
	paramsSize := entry.size()
	if actualArgSize != paramsSize {
		return cmdID, zero, fmt.Errorf("argument size mismatch for cmdID[%d]: expected [%d] bytes, got [%d]", cmdID, paramsSize, actualArgSize)
	}
	var params any
	if actualArgSize > 0 {
		var err error
		params, err = entry.decode(data[InlineHeaderSize:])
		if err != nil {
			return cmdID, zero, fmt.Errorf("decode error: %w", err)
		}
	}
	typedParams, ok := params.(T)
	if !ok {
		return cmdID, zero, fmt.Errorf("type mismatch: expected [%T], got [%T]", zero, params)
	}
	return cmdID, typedParams, nil
}

func Dispatch(ctx *Context) error {
	cmdID := ctx.CmdID
	qsh.Debugf("dispatching callback cmdID[%d]", ctx.CmdID)
	if cmdID >= CmdIDLast {
		return fmt.Errorf("cmdID[%d] is out of bounds", cmdID)
	}
	entry := inlineKeyboardRegistry[cmdID]
	if entry == nil {
		return fmt.Errorf("cmdID[%d] not registered", cmdID)
	}
	handler := entry.handler()
	if handler == nil {
		return fmt.Errorf("no handler for cmdID[%d]", ctx.CmdID)
	}
	return handler(ctx)
}

func NewFactoryFunc[T any]() func() *T {
	return func() *T {
		return new(T)
	}
}

func RegisterStatic[T CallbackParams](cmdID CmdID, factory func() T, handler HandlerFunc) {
	if cmdID == CmdIDUnknown || cmdID >= CmdIDLast {
		panic(fmt.Sprintf("invalid cmdID[%d]", cmdID))
	}
	if inlineKeyboardRegistry[cmdID] != nil {
		panic(fmt.Sprintf("cmdID[%d] already registered", cmdID))
	}
	if factory == nil {
		panic(fmt.Sprintf("cmdID[%d] tried to register a nil factory", cmdID))
	}
	dummy := factory()
	if any(dummy) == nil {
		panic(fmt.Sprintf("cmdID[%d] factory returned a nil instance", cmdID))
	}

	size := dummy.Size()
	if size > MaxInlineKeyboardCallbackDataArgsBytes {
		panic(fmt.Sprintf("cmdID[%d] payload size [%d] exceeds max [%d]", cmdID, size, MaxInlineKeyboardCallbackDataArgsBytes))
	}

	inlineKeyboardRegistry[cmdID] = &callbackEntry[T]{
		paramsSize: size,
		h:          handler,
		make:       factory,
		enc:        func(self T, dst []byte) int { return self.Encode(dst) },
		dec:        func(self T, src []byte) error { return self.Decode(src) },
	}
}

func RegisterAuto[T any](cmdID CmdID, handler TypedHandler[*T]) {
	factory := func() *T { return new(T) }
	dummy := factory()
	size, enc, dec, err := buildTypeCodec(reflect.TypeOf(dummy))
	if err != nil {
		panic(fmt.Errorf("cmdID[%d] auto codec build error: %w", cmdID, err))
	}

	encode := func(self *T, dst []byte) int { return enc(unsafe.Pointer(self), dst) }
	decode := func(self *T, src []byte) error { return dec(unsafe.Pointer(self), src) }

	Register(cmdID, factory, encode, decode, handler)
	if size > MaxInlineKeyboardCallbackDataArgsBytes {
		panic(fmt.Sprintf("cmdID[%d] payload size [%d] exceeds max [%d]", cmdID, size, MaxInlineKeyboardCallbackDataArgsBytes))
	}
}

func buildTypeCodec(t reflect.Type) (int, func(ptr unsafe.Pointer, dst []byte) int, func(ptr unsafe.Pointer, src []byte) error, error) {
	if t == nil {
		return 0, nil, nil, nil
	}
	if t.Kind() != reflect.Pointer {
		return 0, nil, nil, fmt.Errorf("auto codec requires pointer type, got %s", t.Kind())
	}
	innerType := t.Elem()
	return buildFieldCodec(innerType)
}

func buildFieldCodec(t reflect.Type) (int, func(ptr unsafe.Pointer, dst []byte) int, func(ptr unsafe.Pointer, src []byte) error, error) {
	if t.Kind() == reflect.Pointer {
		return 0, nil, nil, fmt.Errorf("pointer types are not supported for automatic codec: %s", t)
	}

	switch t.Kind() {
	case reflect.Struct:
		total := 0
		fields := make([]struct {
			offset uintptr
			size   int
			enc    func(ptr unsafe.Pointer, dst []byte) int
			dec    func(ptr unsafe.Pointer, src []byte) error
		}, 0, t.NumField())

		for field := range t.Fields() {
			if field.PkgPath != "" {
				return 0, nil, nil, fmt.Errorf("unsupported unexported field %s in %s", field.Name, t.Name())
			}
			fieldSize, fieldEnc, fieldDec, err := buildFieldCodec(field.Type)
			if err != nil {
				return 0, nil, nil, fmt.Errorf("struct[%s] field[%s] error: %w", t.Name(), field.Name, err)
			}
			total += fieldSize
			fields = append(fields, struct {
				offset uintptr
				size   int
				enc    func(ptr unsafe.Pointer, dst []byte) int
				dec    func(ptr unsafe.Pointer, src []byte) error
			}{field.Offset, fieldSize, fieldEnc, fieldDec})
		}

		encode := func(ptr unsafe.Pointer, dst []byte) int {
			off := 0
			for _, field := range fields {
				fieldPtr := unsafe.Add(ptr, int(field.offset))
				off += field.enc(fieldPtr, dst[off:])
			}
			return off
		}

		decode := func(ptr unsafe.Pointer, src []byte) error {
			off := 0
			for _, field := range fields {
				fieldPtr := unsafe.Add(ptr, int(field.offset))
				if err := field.dec(fieldPtr, src[off:off+field.size]); err != nil {
					return err
				}
				off += field.size
			}
			return nil
		}

		return total, encode, decode, nil
	case reflect.Array:
		elemSize, elemEnc, elemDec, err := buildFieldCodec(t.Elem())
		if err != nil {
			return 0, nil, nil, fmt.Errorf("array element error: %w", err)
		}
		total := elemSize * t.Len()

		encode := func(ptr unsafe.Pointer, dst []byte) int {
			off := 0
			for i := 0; i < t.Len(); i++ {
				elemPtr := unsafe.Add(ptr, i*elemSize)
				off += elemEnc(elemPtr, dst[off:])
			}
			return off
		}

		decode := func(ptr unsafe.Pointer, src []byte) error {
			off := 0
			for i := 0; i < t.Len(); i++ {
				elemPtr := unsafe.Add(ptr, i*elemSize)
				if err := elemDec(elemPtr, src[off:off+elemSize]); err != nil {
					return err
				}
				off += elemSize
			}
			return nil
		}

		return total, encode, decode, nil
	case reflect.Bool:
		return 1,
			func(ptr unsafe.Pointer, dst []byte) int {
				if *(*bool)(ptr) {
					dst[0] = 1
				} else {
					dst[0] = 0
				}
				return 1
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*bool)(ptr) = src[0] != 0
				return nil
			}, nil
	case reflect.Int8:
		return 1,
			func(ptr unsafe.Pointer, dst []byte) int {
				dst[0] = byte(*(*int8)(ptr))
				return 1
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*int8)(ptr) = int8(src[0])
				return nil
			}, nil
	case reflect.Uint8:
		return 1,
			func(ptr unsafe.Pointer, dst []byte) int {
				dst[0] = *(*uint8)(ptr)
				return 1
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*uint8)(ptr) = src[0]
				return nil
			}, nil
	case reflect.Int16:
		return 2,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint16(dst, uint16(*(*int16)(ptr)))
				return 2
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*int16)(ptr) = int16(binary.BigEndian.Uint16(src))
				return nil
			}, nil
	case reflect.Uint16:
		return 2,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint16(dst, *(*uint16)(ptr))
				return 2
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*uint16)(ptr) = binary.BigEndian.Uint16(src)
				return nil
			}, nil
	case reflect.Int32:
		return 4,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint32(dst, uint32(*(*int32)(ptr)))
				return 4
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*int32)(ptr) = int32(binary.BigEndian.Uint32(src))
				return nil
			}, nil
	case reflect.Uint32:
		return 4,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint32(dst, *(*uint32)(ptr))
				return 4
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*uint32)(ptr) = binary.BigEndian.Uint32(src)
				return nil
			}, nil
	case reflect.Float32:
		return 4,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint32(dst, math.Float32bits(*(*float32)(ptr)))
				return 4
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*float32)(ptr) = math.Float32frombits(binary.BigEndian.Uint32(src))
				return nil
			}, nil
	case reflect.Int64:
		return 8,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint64(dst, uint64(*(*int64)(ptr)))
				return 8
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*int64)(ptr) = int64(binary.BigEndian.Uint64(src))
				return nil
			}, nil
	case reflect.Uint64:
		return 8,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint64(dst, *(*uint64)(ptr))
				return 8
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*uint64)(ptr) = binary.BigEndian.Uint64(src)
				return nil
			}, nil
	case reflect.Float64:
		return 8,
			func(ptr unsafe.Pointer, dst []byte) int {
				binary.BigEndian.PutUint64(dst, math.Float64bits(*(*float64)(ptr)))
				return 8
			},
			func(ptr unsafe.Pointer, src []byte) error {
				*(*float64)(ptr) = math.Float64frombits(binary.BigEndian.Uint64(src))
				return nil
			}, nil
	default:
		return 0, nil, nil, fmt.Errorf("dynamic or unsupported type[%s]", t.Kind())
	}
}

func calcTypeSize(t reflect.Type) (int, error) {
	if t == nil {
		return 0, nil
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	size, _, _, err := buildFieldCodec(t)
	return size, err
}
