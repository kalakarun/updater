# run.sh invocation

The reduced updater runs exactly:

```text
/bin/sh ./run.sh
```

The working directory is the extracted package root. No phase or version arguments are passed. The environment supplies a fixed PATH `/usr/sbin:/usr/bin:/sbin:/bin` and the C locale. Confirm any additional required environment variables before deployment.

The script must:

- Detect the host OS and CPU and install the appropriate files from the single universal archive.
- Return zero only after all installation work succeeds; return nonzero on any failure.
- Run without user input, passwords, confirmations or a controlling terminal. The updater does not connect script stdin; the script must use appropriate unattended options for its OS-specific tools and must not open `/dev/tty`.
- Leave `/opt/cpguard/app/setup/swversion` to the updater.
- Keep installer output free of secrets and within the documented log limits.
- Complete within ten minutes and avoid detached/background installation work.
- Replace running updater binaries by writing temporary files and renaming, if updater replacement is included in the package.

There is no independent running-version or health check. Exit zero is the success criterion requested by the flow. A false successful exit can therefore record a version that was not actually installed.

On an installer failure or an unexpected installer write to swversion, the updater attempts to preserve/restore the original version file and removes disposable temporary files. It does not restore application files. If run.sh needs all-or-nothing behavior, that must be implemented in the script itself.

Power loss after script side effects or between installation and version recording can leave a partially installed application or an old swversion. An I/O or cleanup failure may be reported after application files have changed. Do not interpret every nonzero updater exit as proof that no application changes occurred.

The existing run.sh has not been supplied. Its path, arguments, environment requirements, current version-file behavior, and updater replacement paths still need confirmation. No production installer has been executed.
