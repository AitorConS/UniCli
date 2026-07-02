//go:build !unix

package api

func restrictUnixSocketUmask() func() {
	return func() {}
}
