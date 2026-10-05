module homeproxy/mobile

go 1.26.0

require homeproxy v0.0.0

replace homeproxy => ..

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)

require golang.org/x/mobile v0.0.0-20260908204917-8b95e45f8d3e // indirect

require (
	github.com/quic-go/quic-go v0.54.1 // indirect
	go.uber.org/mock v0.5.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
)
