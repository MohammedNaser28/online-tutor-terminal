package webassets

import "embed"

//go:generate cp -r ../../frontend/. .
//go:embed index.html terminal.html admin.html leaderboard.html static
var FS embed.FS
