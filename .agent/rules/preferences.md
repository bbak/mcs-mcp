---
trigger: always_on
description: General user preferences and coding guidelines for the agent to follow.
---

# User Preferences

- Conceptual integrity is important to me. In case some instruction in our conversation is in conflict with that or some solution that you envisioned is, make me aware of that and ask for explicit confirmation. That means that I care for these three things:
    - **Cohesion** measures how closely related the elements within a single module, class, or component are, ideally focusing on one well-defined purpose. High cohesion means functions and data inside a module work together tightly, improving readability, maintainability, and reusability — for example, a class handling only user authentication. Low cohesion scatters unrelated tasks, complicating debugging and changes.​
    - **Coherence** in programming refers to the logical flow and meaningful organization of code or system components, ensuring they form a unified, understandable whole. It emphasizes that responsibilities align sensibly across modules, similar to high cohesion but broader, promoting robustness and ease of comprehension. Coherent code avoids disjointed logic that confuses developers.
    - **Consistency** ensures uniformity in coding style, naming conventions, architecture, and behavior across a codebase or system. It covers standards like indentation, variable naming (e.g., camelCase throughout), and design patterns, reducing errors and easing collaboration in teams. In distributed systems, it also means all nodes share the same data view.​

- Don't assume, ask. Often, there's a good reason why things are as they are.

- Please apply typical Golang community practices regarding file, variable and functions naming conventions.

- I prefer to use Unix-style line endings in my source files, as stated in my `.editorconfig` file in the root folder. Given that we are on Windows, some tools you use might use Windows-style line endings. Please convert them to Unix-style line endings.

- Given that we are on Windows and the Terminal is Powershell, we don't have Unix tools like `grep` and others available. Use their Powershell counterparts.

- Refer to `docs\charter.md` for the Project Charter.

- Use Conventional Commits for git commit messages with the following prefixes:
    - `feat`: A new feature
    - `fix`: A bug fix
    - `refactor`: Code refactoring
    - `docs`: Documentation changes
    - `chore`: Maintenance, scaffolding, or project setup
    - `build`: Build system changes
