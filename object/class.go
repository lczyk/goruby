package object

// Class is the runtime representation of a ruby class. Instance and
// class-level methods are stored separately; method lookup walks Super
// for inheritance.
type Class struct {
	Name         string
	Super        *Class
	Methods      map[string]RubyMethod // instance methods
	ClassMethods map[string]RubyMethod // singleton-class methods
	Includes     []*Class              // included modules
	Constants    map[string]RubyObject // class / module constants
	ClassVars    map[string]RubyObject // class variables (@@x); shared up the inheritance chain
	IsModule     bool                  // distinguishes module from class (no .new)
	// Version monotonically increments on any method (re)definition on
	// this class. Reserved for future inline call-site caching: a
	// cached (class, method) tuple is valid iff the class's Version
	// hasn't moved since the lookup. Not yet consumed; bump now so the
	// counter is meaningful when caches land.
	Version uint64
}

func NewClass(name string, super *Class) *Class {
	return &Class{
		Name:         name,
		Super:        super,
		Methods:      make(map[string]RubyMethod),
		ClassMethods: make(map[string]RubyMethod),
		Constants:    make(map[string]RubyObject),
		ClassVars:    make(map[string]RubyObject),
	}
}

// AddMethod registers m as the instance method of the given name on c
// and bumps c.Version so any cached call-site lookup is invalidated.
func (c *Class) AddMethod(name string, m RubyMethod) {
	c.Methods[name] = m
	c.Version++
}

// AddClassMethod is the class-method counterpart of AddMethod.
func (c *Class) AddClassMethod(name string, m RubyMethod) {
	c.ClassMethods[name] = m
	c.Version++
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

func (c *Class) Type() Type { return CLASS_OBJ }
func (c *Class) Class() RubyClass {
	if c.IsModule {
		return ModuleClass
	}
	return ClassClass
}
func (c *Class) Inspect() string { return c.Name }

// LookupMethod walks the inheritance + include chain for an instance
// method with the given name.
func (c *Class) LookupMethod(name string) (RubyMethod, bool) {
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
func (c *Class) LookupClassMethod(name string) (RubyMethod, bool) {
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
func (i *Instance) Class() RubyClass { return i.C }
func (i *Instance) Inspect() string  { return "#<" + i.C.Name + ">" }
