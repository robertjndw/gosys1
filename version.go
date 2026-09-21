package sys1

// Version is the current release of this module, reported in the
// User-Agent header of every request.
const Version = "0.1.0"

// userAgent is the base User-Agent value sent with every request. A
// caller-supplied suffix (see WithUserAgent) is appended after it.
const userAgent = "sys1-go/" + Version
