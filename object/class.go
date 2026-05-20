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
	// this class. Inline call-site caches (e.g. spaceshipCache below)
	// compare against Version to detect invalidation.
	Version uint64

	// spaceshipCache memoises the result of LookupMethod("<=>") for
	// this class. Comparable derivations (Object#<, #<=, etc. on
	// ObjectClass) hit this on every comparison in a tight loop; the
	// LookupMethod walk dominates otherwise. spaceshipCacheVersion
	// records the Version observed at cache fill, so any method
	// (re)definition that bumps Version invalidates the entry.
	spaceshipCache        RubyMethod
	spaceshipCacheVersion uint64
	spaceshipCacheValid   bool
}

func NewClass(name string, super *Class) *Class {
	// Default super to Object so user-defined and ad-hoc classes
	// inherit universal methods (send, class, is_a?, ...) via Send's
	// chain walk. ObjectClass / BasicObjectClass set their own super
	// explicitly during package init so this never collides on
	// bootstrap.
	if super == nil && ObjectClass != nil && name != "Object" && name != "BasicObject" {
		super = ObjectClass
	}
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

// LookupSpaceship returns this class's `<=>` method, cached. Cache is
// version-gated against the class's Version counter: any AddMethod /
// AddClassMethod on c (or its ancestors, through the bumped Version)
// invalidates the entry on next call. Returns nil, false when no
// `<=>` exists anywhere in the chain.
func (c *Class) LookupSpaceship() (RubyMethod, bool) {
	if c.spaceshipCacheValid && c.spaceshipCacheVersion == c.Version {
		return c.spaceshipCache, c.spaceshipCache != nil
	}
	m, _ := c.LookupMethod("<=>")
	c.spaceshipCache = m
	c.spaceshipCacheVersion = c.Version
	c.spaceshipCacheValid = true
	return m, m != nil
}

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
