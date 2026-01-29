1.  Configurability: Adding a LOG_PATH or JIRA_LOG_DIR to the `.env` so logs are always found regardless of the host's Current Working Directory.
2.  Clean Output: Disabling ANSI colors in zerolog when running in non-interactive or file-redirected environments to prevent those escape codes ([90m...) from cluttering the logs.

Check: Can File-Logging be disabled, when STDERR is not a console or terminal (or redirected to FILE)?