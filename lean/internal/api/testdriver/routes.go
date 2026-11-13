package testdriver

import "net/http"

func RegisterRoutes(mux *http.ServeMux, sess *Session) {
	mux.HandleFunc("POST /lean/v0/test_driver/state_transition/run", StateTransitionHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/fork_choice/init", sess.ForkChoiceInitHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/fork_choice/step", sess.ForkChoiceStepHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/verify_signatures/run", VerifySignaturesHandler())
}
