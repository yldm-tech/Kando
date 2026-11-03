package media

// Pipeline is the cross-platform boundary for media processing.
// Implementations will be provided by the desktop application and platform
// adapters without leaking OS-specific details into the core package.
type Pipeline interface {
	Download(source string) (string, error)
	Transcribe(videoPath string) (string, error)
	Translate(subtitlePath string, language string) (string, error)
	Export(videoPath string, subtitlePaths ...string) (string, error)
}
