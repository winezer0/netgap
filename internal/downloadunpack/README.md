# downloadunpack

Internal netgap package that composes `downutils` downloads with optional `dezip` extraction. The package stays inside the netgap module while its API and behavior are being validated.

`Run` downloads one item, then either preserves the file or extracts it. `ProcessFile` applies the same extraction rules to an existing local file without downloading it, for callers that perform their own download and integrity checks. `RunMany` processes requests sequentially and records each runtime failure without stopping later requests.

`ExtractAs` accepts `auto`, `zip`, `tar`, `gzip`, and `skip`; an empty value means `skip`. `tar.gz` and other `tar.*` labels map to `tar`. The legacy value `exe` is unsupported and returns an invalid mode error. `auto` preserves ordinary files and extracts recognized archives or compressed streams.

Extraction uses a staging directory and publishes the requested destination only after format and optional `ExpectedFile` validation succeed. The downloaded archive remains unless `RemoveArchiveAfterSuccess` is set. A failed or skipped download never starts extraction.

The download path and extraction destination must not overlap. `Run` rejects a conflicting request before downloading, and `RunMany` prevalidates overlapping paths across the batch before starting any download.
