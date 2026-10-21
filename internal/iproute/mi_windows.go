//go:build windows

package iproute

// Minimal bindings for the Windows SDK MI v1 client ABI (mi.h). Calls stay in
// process; no PowerShell, WinRM listener, COM apartment or external helper is used.
import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	miBoolean  = 0
	miUint8    = 1
	miUint16   = 3
	miUint32   = 5
	miDatetime = 12
	miString   = 13
	miNull     = 1 << 29
)

// MI_Application, MI_Session, MI_Operation and MI_OperationOptions share this ABI.
type miHandle struct {
	reserved1 uint64
	reserved2 uintptr
	ft        *[16]uintptr
}

type miInstance struct{ ft *[18]uintptr }

// MI_Value is a union; MI_Datetime (36 bytes) is its largest member. Round to
// eight-byte alignment, as required by the SDK on Windows amd64/arm64.
type miValue [5]uint64
type miInterval struct{ Days, Hours, Minutes, Seconds, Microseconds, Pad1, Pad2, Pad3 uint32 }

type miClient struct{ app, session miHandle }

func miPtr[T any](p *T) unsafe.Pointer { return unsafe.Pointer(p) }
func miText(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}
func miCall(fn uintptr, args ...any) error {
	values := make([]uintptr, len(args))
	for i, arg := range args {
		switch value := arg.(type) {
		case unsafe.Pointer:
			values[i] = uintptr(value)
		case uintptr:
			values[i] = value
		case int:
			values[i] = uintptr(value)
		case uint32:
			values[i] = uintptr(value)
		default:
			panic("unsupported MI argument")
		}
	}
	code, _, _ := syscall.SyscallN(fn, values...)
	runtime.KeepAlive(args)
	if code != 0 {
		return fmt.Errorf("Windows CIM result %d", code)
	}
	return nil
}

func openMI() (*miClient, error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil, errors.New("native Windows CIM route backend requires a 64-bit build")
	}
	proc := windows.NewLazySystemDLL("mi.dll").NewProc("MI_Application_InitializeV1")
	if err := proc.Find(); err != nil {
		return nil, err
	}
	c := &miClient{}
	if err := miCall(proc.Addr(), 0, 0, 0, miPtr(&c.app)); err != nil {
		return nil, err
	}
	// A null destination selects the local CIM server; a null protocol lets MI
	// choose its local transport, without requiring a configured WinRM service.
	if err := miCall(c.app.ft[1], miPtr(&c.app), 0, 0, 0, 0, 0, miPtr(&c.session)); err != nil {
		miCall(c.app.ft[0], miPtr(&c.app))
		return nil, err
	}
	return c, nil
}

func (c *miClient) close() {
	miCall(c.session.ft[0], miPtr(&c.session), 0, 0)
	miCall(c.app.ft[0], miPtr(&c.app))
}

func (c *miClient) options(ctx context.Context, store string) (*miHandle, error) {
	if store != "" && store != "ActiveStore" && store != "PersistentStore" {
		return nil, errors.New("unsupported Windows route store")
	}
	o := &miHandle{}
	if err := miCall(c.app.ft[5], miPtr(&c.app), 1, miPtr(o)); err != nil {
		return nil, err
	}
	fail := func(err error) (*miHandle, error) { miCall(o.ft[0], miPtr(o)); return nil, err }
	if store != "" {
		text := miText(store)
		v := miValue{}
		*(*uintptr)(unsafe.Pointer(&v)) = uintptr(miPtr(text))
		err := miCall(o.ft[3], miPtr(o), miPtr(miText("PolicyStore")), miString, miPtr(&v), 1, 0)
		runtime.KeepAlive(text)
		if err != nil {
			return fail(err)
		}
	}
	remaining := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	if remaining <= 0 {
		return fail(context.DeadlineExceeded)
	}
	i := miInterval{Seconds: uint32(remaining / time.Second), Microseconds: uint32(remaining % time.Second / time.Microsecond)}
	if err := miCall(o.ft[11], miPtr(o), miPtr(miText("__MI_OPERATIONOPTIONS_TIMEOUT")), miPtr(&i), 0); err != nil {
		return fail(err)
	}
	return o, nil
}

// Consume every result, even after cancellation or a parsing error. MI owns
// borrowed instances until the next pull; callers must copy anything they keep.
func miDrain(ctx context.Context, op *miHandle, visit func(*miInstance) error) error {
	if op.ft == nil {
		return errors.New("Windows CIM did not create an operation")
	}
	defer miCall(op.ft[0], miPtr(op))
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { miCall(op.ft[1], miPtr(op), 0); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	var failure error
	for {
		var instance *miInstance
		var more uint8
		var result uint32
		var message *uint16
		var details *miInstance
		if err := miCall(op.ft[3], miPtr(op), miPtr(&instance), miPtr(&more), miPtr(&result), miPtr(&message), miPtr(&details)); err != nil {
			return errors.Join(failure, fmt.Errorf("pull CIM operation result: %w", err), ctx.Err())
		}
		if result != 0 {
			text := windows.UTF16PtrToString(message)
			if details != nil {
				if extra, err := miGetString(details, "Message"); err == nil && extra != "" && extra != text {
					text += "; " + extra
				}
			}
			failure = errors.Join(failure, fmt.Errorf("CIM provider operation result %d: %s", result, text))
		}
		if instance != nil && failure == nil && ctx.Err() == nil && visit != nil {
			failure = visit(instance)
		}
		if more == 0 {
			return errors.Join(failure, ctx.Err())
		}
	}
}

func (c *miClient) query(ctx context.Context, store, query string, visit func(*miInstance) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o, err := c.options(ctx, store)
	if err != nil {
		return err
	}
	defer miCall(o.ft[0], miPtr(o))
	var op miHandle
	miCall(c.session.ft[8], miPtr(&c.session), 0, miPtr(o), miPtr(miText("root/StandardCimv2")), miPtr(miText("WQL")), miPtr(miText(query)), 0, miPtr(&op))
	return miDrain(ctx, &op, visit)
}

func miGet(i *miInstance, name string, expected uint32) (miValue, error) {
	var v miValue
	var typ, flags uint32
	err := miCall(i.ft[11], miPtr(i), miPtr(miText(name)), miPtr(&v), miPtr(&typ), miPtr(&flags), 0)
	if err == nil && (typ != expected || flags&miNull != 0) {
		err = fmt.Errorf("Windows CIM property %s is absent or has type %d (want %d)", name, typ, expected)
	}
	return v, err
}

func miGetString(i *miInstance, name string) (string, error) {
	v, err := miGet(i, name, miString)
	if err != nil {
		return "", err
	}
	return windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&v))), nil
}

func miGetNumber(i *miInstance, name string, typ uint32) (uint32, error) {
	v, err := miGet(i, name, typ)
	return uint32(v[0]), err
}

func miGetTicks(i *miInstance, name string) (string, error) {
	v, err := miGet(i, name, miDatetime)
	if err != nil {
		return "", err
	}
	parts := (*[9]uint32)(unsafe.Pointer(&v))
	if parts[0] != 0 {
		return "", fmt.Errorf("%s must be a CIM interval", name)
	}
	// Route lifetimes are uint32 seconds; clamp the provider's infinite interval
	// before arithmetic, so an infinite sentinel cannot overflow.
	seconds := uint64(parts[1])*86400 + uint64(parts[2])*3600 + uint64(parts[3])*60 + uint64(parts[4])
	if seconds >= 0xffffffff {
		return "42949672950000000", nil
	}
	return fmt.Sprint(seconds*10000000 + uint64(parts[5])*10), nil
}

type miParameter struct {
	name  string
	typ   uint32
	value miValue
	text  *uint16
}

func miStringParam(name, value string) miParameter {
	p := miParameter{name: name, typ: miString, text: miText(value)}
	*(*uintptr)(unsafe.Pointer(&p.value)) = uintptr(miPtr(p.text))
	return p
}
func miNumberParam(name string, typ uint32, value uint32) miParameter {
	return miParameter{name: name, typ: typ, value: miValue{uint64(value)}}
}

func (c *miClient) parameters(params []miParameter) (*miInstance, error) {
	var input *miInstance
	if err := miCall(c.app.ft[3], miPtr(&c.app), miPtr(miText("__PARAMETERS")), 0, miPtr(&input)); err != nil {
		return nil, fmt.Errorf("construct CIM method parameters: %w", err)
	}
	for j := range params {
		p := &params[j]
		// Match the SDK's dynamic __PARAMETERS example: copy values and
		// leave method direction metadata to the provider schema.
		if err := miCall(input.ft[8], miPtr(input), miPtr(miText(p.name)), miPtr(&p.value), uintptr(p.typ), 0); err != nil {
			miCall(input.ft[2], miPtr(input))
			return nil, fmt.Errorf("set CIM parameter %s (type %d): %w", p.name, p.typ, err)
		}
	}
	runtime.KeepAlive(params)
	return input, nil
}

func (c *miClient) create(ctx context.Context, params []miParameter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := c.parameters(params)
	if err != nil {
		return err
	}
	defer miCall(input.ft[2], miPtr(input))
	o, err := c.options(ctx, "")
	if err != nil {
		return err
	}
	defer miCall(o.ft[0], miPtr(o))
	var op miHandle
	miCall(c.session.ft[6], miPtr(&c.session), 0, miPtr(o), miPtr(miText("root/StandardCimv2")), miPtr(miText("MSFT_NetRoute")), miPtr(miText("Create")), 0, miPtr(input), 0, miPtr(&op))
	seen := false
	err = miDrain(ctx, &op, func(i *miInstance) error {
		code, err := miGetNumber(i, "ReturnValue", miUint32)
		if err != nil {
			return err
		}
		seen = true
		if code != 0 {
			return fmt.Errorf("MSFT_NetRoute.Create returned %d", code)
		}
		return nil
	})
	if err == nil && !seen {
		return errors.New("MSFT_NetRoute.Create returned no completion value")
	}
	return err
}

func (c *miClient) delete(ctx context.Context, store string, instance *miInstance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o, err := c.options(ctx, store)
	if err != nil {
		return err
	}
	defer miCall(o.ft[0], miPtr(o))
	var op miHandle
	miCall(c.session.ft[5], miPtr(&c.session), 0, miPtr(o), miPtr(miText("root/StandardCimv2")), miPtr(instance), 0, miPtr(&op))
	return miDrain(ctx, &op, nil)
}
