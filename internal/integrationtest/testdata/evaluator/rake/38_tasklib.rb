# minversion: 2.6
# Exercise Rake::TaskLib -- the base class rake exposes for user-
# defined task libraries (TestTask, PackageTask, etc. inherit from
# it). A TaskLib is just a class that mixes in DSL + Cloneable, so
# instances can call task / file / desc directly and the tasks
# land in the global rake registry.

$LOAD_PATH.unshift __dir__ + "/../../gems/rake/lib"

require "rake/tasklib"

class MyLib < Rake::TaskLib
  def initialize(prefix)
    @prefix = prefix
    define
  end

  def define
    task "#{@prefix}:setup" do
      puts "setup-#{@prefix}"
    end

    task "#{@prefix}:run" => "#{@prefix}:setup" do
      puts "run-#{@prefix}"
    end
  end
end

MyLib.new("alpha")
MyLib.new("beta")

# Tasks defined via the lib are reachable through the registry.
puts Rake::Task["alpha:setup"].name           #=> alpha:setup
puts Rake::Task["beta:run"].name              #=> beta:run

# Prerequisites composed via define are preserved.
puts Rake::Task["alpha:run"].prerequisites.first   #=> alpha:setup

# Invoking the leaf walks back through the prereq.
Rake::Task["beta:run"].invoke
                                              #=> setup-beta
                                              #=> run-beta
