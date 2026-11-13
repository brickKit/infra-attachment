// Command infra-attachment runs the attachment component alone (serve, migrate); the shells compile in
// attachment.Definition().
package main

import (
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/infra-attachment/v3/attachment"
)

func main() { launcher.Main(attachment.Definition()) }
