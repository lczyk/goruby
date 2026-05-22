package stdlib

import (
	"time"

	"github.com/lczyk/goruby/evaluator/builtinapi"
	"github.com/lczyk/goruby/object"
)

// BootstrapDateClass installs a minimal stdlib `Date` class. Covers
// what corpus uses today: constructor, year/month/day accessors,
// arithmetic with Integer (days), to_s ("YYYY-MM-DD"), and a small
// subset of strftime directives.
//
// require 'date' still routes through Kernel#require's allowlist;
// this bootstrap runs eagerly so the constant is available regardless.
func BootstrapDateClass(env *object.Environment) *object.Class {
	if existing, ok := env.Get("Date"); ok {
		if c, ok := existing.(*object.Class); ok {
			return c
		}
	}
	c := object.NewClass("Date", nil)
	c.ClassMethods["new"] = &object.UserMethod{Name: "new", Body: builtinapi.NativeFn{Fn: dateNew(c)}}
	c.Methods["year"] = &object.BuiltinMethod{Name: "year", Fn: dateAccessor("year")}
	c.Methods["month"] = &object.BuiltinMethod{Name: "month", Fn: dateAccessor("month")}
	c.Methods["mon"] = c.Methods["month"]
	c.Methods["day"] = &object.BuiltinMethod{Name: "day", Fn: dateAccessor("day")}
	c.Methods["mday"] = c.Methods["day"]
	c.Methods["+"] = &object.BuiltinMethod{Name: "+", Fn: dateAdd(c, +1)}
	c.Methods["-"] = &object.BuiltinMethod{Name: "-", Fn: dateAdd(c, -1)}
	c.Methods["to_s"] = &object.BuiltinMethod{Name: "to_s", Fn: dateToS}
	c.Methods["inspect"] = &object.BuiltinMethod{Name: "inspect", Fn: dateInspect}
	c.Methods["strftime"] = &object.BuiltinMethod{Name: "strftime", Fn: dateStrftime}
	c.Methods["wday"] = &object.BuiltinMethod{Name: "wday", Fn: func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		t, ok := dateTime(recv)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Date#wday on non-Date %T", recv)
		}
		return object.NewInteger(int64(t.Weekday())), nil
	}}
	env.SetGlobal("Date", c)
	return c
}

func dateNew(c *object.Class) func(*object.Environment, []object.RubyObject) (object.RubyObject, error) {
	return func(env *object.Environment, args []object.RubyObject) (object.RubyObject, error) {
		y, m, d := 1, 1, 1
		if len(args) >= 1 {
			i, ok := args[0].(*object.Integer)
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Date.new: year must be Integer, got %T", args[0])
			}
			y = int(i.Value)
		}
		if len(args) >= 2 {
			i, ok := args[1].(*object.Integer)
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Date.new: month must be Integer, got %T", args[1])
			}
			m = int(i.Value)
		}
		if len(args) >= 3 {
			i, ok := args[2].(*object.Integer)
			if !ok {
				return nil, builtinapi.Errorf("evaluator: Date.new: day must be Integer, got %T", args[2])
			}
			d = int(i.Value)
		}
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return newDateInstance(c, t), nil
	}
}

func newDateInstance(c *object.Class, t time.Time) *object.Instance {
	return &object.Instance{
		C: c,
		Ivars: map[string]object.RubyObject{
			"@year":  object.NewInteger(int64(t.Year())),
			"@month": object.NewInteger(int64(t.Month())),
			"@day":   object.NewInteger(int64(t.Day())),
		},
	}
}

func dateTime(recv object.RubyObject) (time.Time, bool) {
	inst, ok := recv.(*object.Instance)
	if !ok {
		return time.Time{}, false
	}
	y, ok1 := inst.Ivars["@year"].(*object.Integer)
	m, ok2 := inst.Ivars["@month"].(*object.Integer)
	d, ok3 := inst.Ivars["@day"].(*object.Integer)
	if !ok1 || !ok2 || !ok3 {
		return time.Time{}, false
	}
	return time.Date(int(y.Value), time.Month(m.Value), int(d.Value), 0, 0, 0, 0, time.UTC), true
}

func dateAccessor(part string) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	ivar := "@" + part
	if part == "month" || part == "mon" {
		ivar = "@month"
	}
	return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		inst, ok := recv.(*object.Instance)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Date#%s on non-Date %T", part, recv)
		}
		if v, ok := inst.Ivars[ivar]; ok {
			return v, nil
		}
		return object.NewInteger(0), nil
	}
}

func dateAdd(c *object.Class, sign int) func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	return func(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
		if len(args) != 1 {
			return nil, builtinapi.Errorf("evaluator: Date arithmetic expects 1 arg")
		}
		days, ok := args[0].(*object.Integer)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Date arithmetic with non-Integer (%T) not supported", args[0])
		}
		t, ok := dateTime(recv)
		if !ok {
			return nil, builtinapi.Errorf("evaluator: Date arithmetic on non-Date %T", recv)
		}
		shifted := t.AddDate(0, 0, sign*int(days.Value))
		return newDateInstance(c, shifted), nil
	}
}

func dateToS(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	t, ok := dateTime(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Date#to_s on non-Date %T", recv)
	}
	return object.NewString(t.Format("2006-01-02")), nil
}

func dateInspect(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	t, ok := dateTime(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Date#inspect on non-Date %T", recv)
	}
	return object.NewString("#<Date: " + t.Format("2006-01-02") + ">"), nil
}

// dateStrftime supports a useful subset of MRI strftime directives:
// %Y, %m, %d, %A (weekday name), %a (short weekday), %B (month name),
// %b (short month), %j (day-of-year), %H/%M/%S (zero for plain Date),
// %% (literal %). Unknown directives pass through unchanged.
func dateStrftime(env *object.Environment, recv object.RubyObject, args []object.RubyObject, block any) (object.RubyObject, error) {
	if len(args) != 1 {
		return nil, builtinapi.Errorf("evaluator: Date#strftime expects 1 arg")
	}
	fmt, ok := builtinapi.StringText(env, args[0])
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Date#strftime expects String format")
	}
	t, ok := dateTime(recv)
	if !ok {
		return nil, builtinapi.Errorf("evaluator: Date#strftime on non-Date %T", recv)
	}
	var out []byte
	for i := 0; i < len(fmt); i++ {
		c := fmt[i]
		if c != '%' || i+1 >= len(fmt) {
			out = append(out, c)
			continue
		}
		i++
		switch fmt[i] {
		case 'Y':
			out = append(out, []byte(t.Format("2006"))...)
		case 'm':
			out = append(out, []byte(t.Format("01"))...)
		case 'd':
			out = append(out, []byte(t.Format("02"))...)
		case 'A':
			out = append(out, []byte(t.Weekday().String())...)
		case 'a':
			out = append(out, []byte(t.Weekday().String()[:3])...)
		case 'B':
			out = append(out, []byte(t.Month().String())...)
		case 'b':
			out = append(out, []byte(t.Month().String()[:3])...)
		case 'j':
			out = append(out, []byte(t.Format("002"))...)
		case 'H', 'M', 'S':
			out = append(out, '0', '0')
		case '%':
			out = append(out, '%')
		default:
			out = append(out, '%', fmt[i])
		}
	}
	return object.NewString(string(out)), nil
}
