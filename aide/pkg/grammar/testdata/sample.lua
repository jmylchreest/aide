-- Sample Lua module for grammar testing.

local M = {}

local function fibonacci(n)
    if n <= 1 then
        return n
    end
    return fibonacci(n - 1) + fibonacci(n - 2)
end

local function factorial(n)
    if n == 0 then
        return 1
    end
    return n * factorial(n - 1)
end

function M.greet(name)
    print("Hello, " .. name)
end

function M.compute(n)
    return {
        fib = fibonacci(n),
        fact = factorial(n),
    }
end

return M
