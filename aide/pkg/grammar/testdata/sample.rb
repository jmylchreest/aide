# Sample Ruby script for grammar testing.

module Greeter
  def self.greet(name)
    puts "Hello, #{name}!"
  end
end

class Calculator
  def initialize(precision = 2)
    @precision = precision
  end

  def factorial(n)
    return 1 if n <= 1
    n * factorial(n - 1)
  end

  def fibonacci(n)
    return n if n <= 1
    fibonacci(n - 1) + fibonacci(n - 2)
  end

  def format(value)
    value.round(@precision)
  end
end

def main
  Greeter.greet("world")
  calc = Calculator.new
  puts "5! = #{calc.factorial(5)}"
  puts "fib(10) = #{calc.fibonacci(10)}"
end

main
