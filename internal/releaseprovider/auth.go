package releaseprovider

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrSecureStoreUnavailable = errors.New("approved operating-system credential store is unavailable")

const credentialKey = "maestro/private-release/github-app"

type SecureStore interface {
	Available() error
	Get(string) ([]byte, error)
	Put(string, []byte) error
	Delete(string) error
}

type UnavailableStore struct{}

func (UnavailableStore) Available() error           { return ErrSecureStoreUnavailable }
func (UnavailableStore) Get(string) ([]byte, error) { return nil, ErrSecureStoreUnavailable }
func (UnavailableStore) Put(string, []byte) error   { return ErrSecureStoreUnavailable }
func (UnavailableStore) Delete(string) error        { return ErrSecureStoreUnavailable }

type AuthService struct {
	Flow  DeviceFlowClient
	Store SecureStore
}

type AuthStatus struct {
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Refresh   bool      `json:"refresh_available"`
}

func (service AuthService) Login(ctx context.Context, present func(DeviceAuthorization) error) (AuthStatus, error) {
	if service.Store == nil {
		return AuthStatus{}, ErrSecureStoreUnavailable
	}
	if err := service.Store.Available(); err != nil {
		return AuthStatus{}, err
	}
	authorization, err := service.Flow.Begin(ctx)
	if err != nil {
		return AuthStatus{}, err
	}
	if present != nil {
		if err := present(DeviceAuthorization{
			UserCode: authorization.UserCode, VerificationURI: authorization.VerificationURI,
			ExpiresAt: authorization.ExpiresAt, Interval: authorization.Interval,
		}); err != nil {
			return AuthStatus{}, err
		}
	}
	token, err := service.Flow.Poll(ctx, authorization)
	if err != nil {
		return AuthStatus{}, err
	}
	if err := service.save(token); err != nil {
		return AuthStatus{}, err
	}
	return statusFor(token, service.Flow.now()), nil
}

func (service AuthService) Status() (AuthStatus, error) {
	token, err := service.load()
	if err != nil {
		return AuthStatus{}, err
	}
	return statusFor(token, service.Flow.now()), nil
}

func (service AuthService) AccessToken(ctx context.Context) (string, error) {
	token, err := service.load()
	if err != nil {
		return "", err
	}
	if service.Flow.now().Add(time.Minute).Before(token.ExpiresAt) {
		return token.AccessToken, nil
	}
	refreshed, err := service.Flow.Refresh(ctx, token.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := service.save(refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func (service AuthService) Logout() error {
	if service.Store == nil {
		return ErrSecureStoreUnavailable
	}
	if err := service.Store.Available(); err != nil {
		return err
	}
	return service.Store.Delete(credentialKey)
}

func (service AuthService) load() (Token, error) {
	if service.Store == nil {
		return Token{}, ErrSecureStoreUnavailable
	}
	if err := service.Store.Available(); err != nil {
		return Token{}, err
	}
	body, err := service.Store.Get(credentialKey)
	if err != nil {
		return Token{}, err
	}
	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return Token{}, errors.New("stored provider credential is invalid")
	}
	if token.AccessToken == "" {
		return Token{}, errors.New("stored provider credential is incomplete")
	}
	return token, nil
}

func (service AuthService) save(token Token) error {
	body, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return service.Store.Put(credentialKey, body)
}

func statusFor(token Token, now time.Time) AuthStatus {
	state := "authenticated"
	if !now.Before(token.ExpiresAt) {
		state = "refresh_required"
	}
	return AuthStatus{State: state, ExpiresAt: token.ExpiresAt, Refresh: token.RefreshToken != ""}
}
