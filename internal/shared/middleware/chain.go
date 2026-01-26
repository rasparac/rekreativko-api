package middleware

import "net/http"

type Chain struct {
	middlewares []func(http.Handler) http.Handler
}

func NewChain(middlewares ...func(http.Handler) http.Handler) *Chain {
	return &Chain{
		middlewares: middlewares,
	}
}

func (c *Chain) Then(h http.Handler) http.Handler {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}
	return h
}

func (c *Chain) ThenFunc(fn http.HandlerFunc) http.Handler {
	return c.Then(fn)
}

func (c *Chain) Append(middlewares ...func(http.Handler) http.Handler) *Chain {
	newMiddlewares := make([]func(http.Handler) http.Handler, len(c.middlewares)+len(middlewares))

	copy(newMiddlewares, c.middlewares)
	copy(newMiddlewares[len(c.middlewares):], middlewares)

	return &Chain{
		middlewares: newMiddlewares,
	}
}

func (c *Chain) Extend(middlewares ...func(http.Handler) http.Handler) *Chain {
	return c.Append(middlewares...)
}
