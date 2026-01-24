# Project Charter

## Project Name

MCS MCP

## Project Description

Given that LLM's are not particularly good at Forcasting, yet Project Managers might use AI Tooling to do their Job, I want to provide them with a MCP-Server that provides Methods to do proper Forecasting, the results of which shall provided to the AI, so it can incorporate that in it's answers to a Project Manager.

The forecasting shall be done by Monte-Carlo-Simulation (MCS) as well as additional statictical means to hint at the validity of the underlying data, like long/fat-tail analysis on the throughput histogram.

Obviously, a random smapling technique like MCS requires historic data. That resides in Atlassian Jira and shall be retrieved by using Atlassian's REST API to fetch work items from it. Given that one of the most cricial parts of MCS is the data selection for sampling, we likely need a flexible, yet multi-step process of interaction between the MCP Server, the AI and the User to clarify things like the Jira Board or Filter to use, which issuetypes, timeframe and the like. During that process the MCP Server shall provide additional information, based on preliminary data-analysis to the AI, so it can guide the User through the process of selecting the right data for sampling.

## Technical Stack & Constraints

- Golang: Given that statictical analysis and simulation is a CPU-intensive task, we use Golang.
- Atlassian Jira: Use native HTTP/JSON API to fetch work items from it, primarily using the `/search` endpoint.
    - For the time being, we need an `.env` file to store the Jira Authentication data. Sadly, we need to use Cookie based Authentication, later on extending to Personal Access Tokens.
- Logging: Use `zerolog` for logging. Plan for a `--verbose` flag to enable verbose logging.
- Use a proper tool like `cobra` to handle commandline parameters when starting the MCP Server.
- Create Unit tests and integration tests where possible and use `go test` to run them.
- Use `golangci-lint` to run static code analysis.
-

## Documentation

- Given that the target audience of this project are Project Managers in mid- and large-size companies, we need to provide them with a clear and concise documentation on how to use the MCP Server and a thorough technical documentation for the MCP Server itself. The documentation shall be available in the `docs` directory of the repository, in markdown format.
- Archtectural decisions, primary design principles and concepts shall be documented in the `docs/architecture.md` file of the repository, in markdown format.
