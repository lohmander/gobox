# GoBox: Timeboxing with Git Accountability

## 🚀 What is GoBox?

`GoBox` is a command-line interface (CLI) tool built in Go. Its purpose is to assist developers and individuals in applying **timeboxing** directly within their Markdown-based task lists. It integrates with your Git repository to provide a record of activity during timeboxed sessions.

The core idea is to allocate focused time blocks to specific tasks. `GoBox` then automatically records the Git commits made during that period, offering a way to track progress and maintain focus.

## ✨ Goals

* **Improve Focus:** Support dedicated work on a single task for a set duration.
* **Track Accountability:** Provide a record of Git commits associated with timeboxed sessions.
* **Manage Scope:** The visibility of commits within a timebox can help in staying focused on the task's defined scope.
* **Integrate Workflow:** Connect task management with existing Markdown notes and Git practices.

## 🌟 High-Level Features

* **Markdown Checklist Parsing:** Reads Markdown files to identify checklist items (`- [ ]` or `- [x]`).
* **Timebox Syntax Recognition:** Interprets timebox definitions per task, such as `@1h30m` (for durations) or `@[10:00-12:00]` (for specific time ranges).
* **Interactive Timer:** Initiates a timer for the next available task, counting down until completion or user input.
* **Git Integration:** Monitors the local Git repository and displays new commits as they occur during the active timebox.
* **Automated Markdown Update:** Upon task completion, `GoBox` performs the following updates to the Markdown file:
  * Checks the task's box (`- [x]`).
  * Appends a completion timestamp and the actual duration spent.
  * Lists all Git commits made during the timeboxed session directly under the task.

* **CLI Argument Handling:** Utilizes `spf13/cobra` for processing command-line arguments.

## 🛠️ Installation (Coming Soon)

*(Once the project is more mature, this section will contain detailed installation instructions, likely involving `go install` or `make install`.)*

## 🚀 Usage (Coming Soon)

*(This section will provide simple command examples once the core features are fully integrated and stable.)*

## 🛣️ Future Enhancements

Planned enhancements for `GoBox` include:

* Improved console UI with `charmbracelet/bubbles` (e.g., progress bars, dynamic commit lists).
* Interactive task selection from the terminal.
* Support for resuming interrupted sessions (persistent state).
* Enhanced parsing for nested tasks in Markdown.
* Visual indicators for overdue tasks.
* Optional audio cues for timer completion.
* Potential macOS status bar integration.
