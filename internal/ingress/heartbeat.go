package ingress

import "context"

func (g *Gateway) SessionPathForKey(_ context.Context, sessionKey string) (string, error) {
	state, err := g.loadState()
	if err != nil {
		return "", err
	}
	return state.Sessions[sessionKey], nil
}
