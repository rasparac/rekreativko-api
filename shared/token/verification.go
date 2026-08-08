package token

import (
	"crypto/rand"
	"math/big"
)

type VerificationCodeGenerator struct {
	length int
}

func NewVerificationCodeGenerator(length int) *VerificationCodeGenerator {
	return &VerificationCodeGenerator{
		length: length,
	}
}

func (g *VerificationCodeGenerator) Generate() (string, error) {
	code := ""

	for i := 0; i < g.length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}

		code += num.String()
	}

	return code, nil
}
