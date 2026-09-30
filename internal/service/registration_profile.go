package service

import "clientesFrecuentes/internal/identitycode"

type SignupProfileError struct {
	Name                string
	LastName            string
	AccountTypeRequired bool
}

func (e *SignupProfileError) Unwrap() error {
	if e.AccountTypeRequired {
		return ErrAccountTypeRequired
	}
	return ErrRegistrationProfileRequired
}
func (e *SignupProfileError) Error() string { return e.Unwrap().Error() }

func registrationNames(name, lastName string) (string, string, error) {
	n, e1 := cleanName(name, 120)
	l, e2 := cleanName(lastName, 120)
	if e1 != nil || e2 != nil {
		return "", "", ErrRegistrationProfileRequired
	}
	if _, ok := identitycode.Prefix(n, l); !ok {
		return "", "", ErrRegistrationProfileRequired
	}
	return n, l, nil
}
