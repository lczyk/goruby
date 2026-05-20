package object

// Send dispatches `name` on `recv`. It walks recv.Class()'s ancestry,
// invokes the first matching RubyMethod, or falls through to a
// method_missing definition if one exists somewhere in the chain.
//
// found=false means no method (and no method_missing) was found --
// callers in the legacy dispatcher use that signal to fall back to the
// hand-rolled type switch during the in-progress migration. Once all
// builtin methods live on their class, the legacy path goes away and
// found=false simply propagates as NoMethodError.
func Send(env *Environment, recv RubyObject, name string, args []RubyObject, block any) (result RubyObject, found bool, err error) {
	cls := recv.Class()
	if cls == nil {
		return nil, false, nil
	}
	if m, ok := cls.LookupMethod(name); ok {
		v, e := m.Call(env, recv, args, block)
		return v, true, e
	}
	if mm, ok := cls.LookupMethod("method_missing"); ok {
		mmArgs := make([]RubyObject, 0, 1+len(args))
		mmArgs = append(mmArgs, env.Symbols().Intern(name))
		mmArgs = append(mmArgs, args...)
		v, e := mm.Call(env, recv, mmArgs, block)
		return v, true, e
	}
	return nil, false, nil
}
