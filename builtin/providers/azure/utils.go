package azure

import (
	"math/rand"
	"strings"
)

// reverseDNSName simply reverses the provided DNS name.
func reverseDNSName(dnsName string) string {
	bits := strings.Split(dnsName, ".")
	// reverse the bits:
	for i, j := 0, len(bits)-1; i < j; i, j = i+1, j-1 {
		bits[i], bits[j] = bits[j], bits[i]
	}
	return strings.Join(bits, ".")
}

// getRandomStringLabel returns a random string of the given length.
func getRandomStringLabel(n int) string {
	var chars = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	buf := make([]rune, n)
	for i := 0; i < n; i++ {
		buf[i] = chars[rand.Intn(len(chars))]
	}
	return string(buf)
}

// sprintfParams is a helper function which takes a string-bool map and returns
// a formatted string with all the keys for displaying in errors.
func sprintfParams(m map[string]bool) string {
	s := ""
	for k, _ := range m {
		s = s + k + ", "
	}
	return s[:len(s)-2]
}
