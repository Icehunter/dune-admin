// ConnState distinguishes the initial load from a hard "never reached the
// backend" failure, so the UI can show a setup screen on real connection
// failure without flickering during the first poll.
export type ConnState = 'loading' | 'connected' | 'error'
