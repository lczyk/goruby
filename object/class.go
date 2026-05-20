package object

// Class is the runtime representation of a ruby class. Instance and
// class-level methods are stored separately; method lookup walks Super
// for inheritance.
type Class struct {
	Name         string
	Super        *Class
	Methods      map[string]RubyObject // instance methods (UserMethod values)
	ClassMethods map[string]RubyObject // singleton-class methods
	Includes     []*Class              // included modules
	Constants    map[string]RubyObject // class / module constants
	ClassVars    map[string]RubyObject // class variables (@@x); shared up the inheritance chain
	IsModule     bool                  // distinguishes module from class (no .new)
}

func NewClass(name string, super *Class) *Class {
	return &Class{
		Name:         name,
		Super:        super,
		Methods:      make(map[string]RubyObject),
		ClassMethods: make(map[string]RubyObject),
		Constants:    make(map[string]RubyObject),
		ClassVars:    make(map[string]RubyObject),
	}
}

// LookupClassVar walks the super chain for a class variable (matches
// MRI: cvars are shared across the inheritance tree).
func (c *Class) LookupClassVar(name string) (RubyObject, *Class) {
	for cur := c; cur != nil; cur = cur.Super {
		if v, ok := cur.ClassVars[name]; ok {
			return v, cur
		}
	}
	return nil, nil
}

func (c *Class) Type() Type       { return CLASS_OBJ }
func (c *Class) Class() RubyClass { return nil }
func (c *Class) Inspect() string  { return c.Name }

// LookupMethod walks the inheritance + include chain for an instance
// method with the given name.
func (c *Class) LookupMethod(name string) (RubyObject, bool) {
	for cur := c; cur != nil; cur = cur.Super {
		if m, ok := cur.Methods[name]; ok {
			return m, true
		}
		for _, inc := range cur.Includes {
			if m, ok := inc.LookupMethod(name); ok {
				return m, true
			}
		}
	}
	return nil, false
}

// LookupClassMethod walks the chain for a class-level method.
func (c *Class) LookupClassMethod(name string) (RubyObject, bool) {
	for cur := c; cur != nil; cur = cur.Super {
		if m, ok := cur.ClassMethods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

// IsAncestor reports whether other appears in c's inheritance / include
// chain (inclusive).
func (c *Class) IsAncestor(other *Class) bool {
	for cur := c; cur != nil; cur = cur.Super {
		if cur == other {
			return true
		}
		for _, inc := range cur.Includes {
			if inc.IsAncestor(other) {
				return true
			}
		}
	}
	return false
}

// Instance is a ruby object instantiated from a Class.
type Instance struct {
	C     *Class
	Ivars map[string]RubyObject
}

func NewInstance(c *Class) *Instance {
	return &Instance{C: c, Ivars: make(map[string]RubyObject)}
}

func (i *Instance) Type() Type       { return OBJECT_OBJ }
func (i *Instance) Class() RubyClass { return nil }
func (i *Instance) Inspect() string  { return "#<" + i.C.Name + ">" }
