package sites

type PlaywrightScriptData struct {
	Engine        string
	Query         string
	ProfileDir    string
	Channel       string
	Login         bool
	MaxResults    int
	TimeoutMS     int64
	HeadlessMode  string
	Snapshot      bool
	SnapshotStamp string
	ProjectRoot   string
	OutputPath    string
}
