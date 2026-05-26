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
	// Private records instance-method names that may not be called via
	// an explicit receiver. Populated by `private` inside a class body.
	Private map[string]bool
	// Protected records instance-method names callable via explicit
	// receiver only from inside an instance of the same class (or a
	// subclass). Populated by `protected` inside a class body.
	Protected map[string]bool
	// CurrentVisibility is the visibility mode the next method def in
	// this class body will inherit. "public" by default; flipped by
	// bare `private` / `public` keywords; transient -- meaningful only
	// while the class body is open.
	CurrentVisibility string
	// Version monotonically increments on any method (re)definition on
	// this class. Inline call-site caches (e.g. spaceshipCache below)
	// compare against Version to detect invalidation.
	Version uint64

	// Parent is the enclosing module/class this class was defined inside,
	// when known. Used by QualifiedName / Inspect to render the dotted
	// path (Outer::Inner). nil for top-level classes and for classes
	// whose enclosure was not tracked at creation time.
	Parent *Class

	// Ivars holds class-level instance variables -- @var assigned at
	// class-body scope (where self == the class object), or via
	// `def self.foo; @var = ...; end`. Distinct from ClassVars (@@x),
	// which are shared up the inheritance chain; class-instance vars
	// belong to one class object only.
	Ivars map[string]RubyObject

	// SingletonClass is the per-class metaclass. Materialised on first
	// access (typically when `class << SomeClass; ...; end` opens the
	// eigenclass and code inside reads `self`). Methods installed on
	// it are class-level methods of the host, distinct from ClassMethods
	// only in that they survive the singleton-class-as-value idiom
	// `(class << self; self; end).attr_accessor :foo`. Send / dispatch
	// for Class receivers consult ClassSingleton.Methods alongside
	// ClassMethods.
	ClassSingleton *Class

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
func (c *Class) Inspect() string { return c.QualifiedName() }

// QualifiedName walks Parent and returns the dotted path
// (Outer::Inner::Leaf). Falls back to bare Name when Parent is nil.
func (c *Class) QualifiedName() string {
	if c.Parent == nil || c.Parent.Name == "" {
		return c.Name
	}
	return c.Parent.QualifiedName() + "::" + c.Name
}

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

// EnsureClassSingleton returns c's per-class metaclass, lazy-
// creating it on first call. Methods installed on the singleton
// behave as class methods on c, with the eigenclass-as-value
// idiom `(class << self; self; end).attr_accessor name` working
// because the value handed out is a real Class.
func (c *Class) EnsureClassSingleton() *Class {
	if c.ClassSingleton == nil {
		sc := NewClass("#<Class:"+c.QualifiedName()+">", nil)
		sc.IsModule = true
		c.ClassSingleton = sc
	}
	return c.ClassSingleton
}

// LookupMethod walks the inheritance + include chain for an instance
// method with the given name. Mirrors MRI's MRO: own Methods, then
// each Include (only the include's own Methods + its further
// Includes; the include's Super chain is NOT followed, since that
// would route Object methods ahead of the receiver's real Super
// and break method-resolution order).
func (c *Class) LookupMethod(name string) (RubyMethod, bool) {
	for cur := c; cur != nil; cur = cur.Super {
		if m, ok := cur.Methods[name]; ok {
			return m, true
		}
		for _, inc := range cur.Includes {
			if m, ok := inc.lookupOwnAndIncludes(name); ok {
				return m, true
			}
		}
	}
	return nil, false
}

// lookupOwnAndIncludes returns name from this class's Methods or any
// of its (recursive) Includes' Methods, but does NOT follow Super.
// Used by LookupMethod when walking the includes chain so the
// includes only contribute their own contents to the MRO.
func (c *Class) lookupOwnAndIncludes(name string) (RubyMethod, bool) {
	if c == nil {
		return nil, false
	}
	if m, ok := c.Methods[name]; ok {
		return m, true
	}
	for _, inc := range c.Includes {
		if m, ok := inc.lookupOwnAndIncludes(name); ok {
			return m, true
		}
	}
	return nil, false
}

// LookupClassMethod walks the chain for a class-level method.
// Consults each class's ClassSingleton (singleton-class instance
// methods land here under the eigenclass-as-value idiom) before
// its ClassMethods table.
func (c *Class) LookupClassMethod(name string) (RubyMethod, bool) {
	for cur := c; cur != nil; cur = cur.Super {
		if cur.ClassSingleton != nil {
			if m, ok := cur.ClassSingleton.Methods[name]; ok {
				return m, true
			}
		}
		if m, ok := cur.ClassMethods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

// IsAncestor reports whether other appears in c's inheritance / include
// chain (inclusive). Cycle-safe: a visited set guards against include
// loops (e.g. minitest/spec's class-of-class self-reference shapes).
func (c *Class) IsAncestor(other *Class) bool {
	visited := map[*Class]bool{}
	return c.isAncestorVisit(other, visited)
}

func (c *Class) isAncestorVisit(other *Class, visited map[*Class]bool) bool {
	for cur := c; cur != nil; cur = cur.Super {
		if visited[cur] {
			return false
		}
		visited[cur] = true
		if cur == other {
			return true
		}
		for _, inc := range cur.Includes {
			if inc.isAncestorVisit(other, visited) {
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
	// SingletonMethods holds per-object method definitions installed by
	// `def obj.foo; ... end`. Consulted by Send before the class chain
	// so a singleton method overrides the class definition.
	SingletonMethods map[string]RubyMethod
	// SingletonClass is the per-object metaclass, lazily materialised
	// on first `singleton_class` call. Its Super is the object's
	// regular class; Send consults its method chain before falling
	// through to C. Distinct from SingletonMethods only in that
	// methods installed via define_method on the singleton class land
	// in SingletonClass.Methods rather than the SingletonMethods map.
	SingletonClass *Class
}

// EnsureSingletonClass returns the object's per-object metaclass,
// creating it on first call. Super chains up to the object's regular
// class so inherited methods stay reachable.
func (i *Instance) EnsureSingletonClass() *Class {
	if i.SingletonClass == nil {
		sc := NewClass("#<Class:#<"+i.C.Name+">>", i.C)
		i.SingletonClass = sc
	}
	return i.SingletonClass
}

func NewInstance(c *Class) *Instance {
	return &Instance{C: c, Ivars: make(map[string]RubyObject)}
}

func (i *Instance) Type() Type       { return OBJECT_OBJ }
func (i *Instance) Class() RubyClass { return i.C }
func (i *Instance) Inspect() string  { return "#<" + i.C.Name + ">" }
