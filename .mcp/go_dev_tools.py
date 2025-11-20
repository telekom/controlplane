# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

import json
import os
import subprocess
from pathlib import Path
from typing import Dict, List, Any

from fastmcp import FastMCP

mcp = FastMCP("go-dev-tools")

# =============================================================================
# GO DEVELOPMENT TOOLS WORKFLOW
# =============================================================================
"""
This MCP server provides comprehensive Go development tools including linting,
code quality analysis, coverage reporting, and module management:

DEVELOPMENT WORKFLOW:
1. 🔍 MODULE DISCOVERY
   - Use find_modules() to discover all available Go modules in the project
   - Use find_staged_modules() to identify only modules with staged changes
   - Use get_info_about_module() to understand module structure and purpose
   - Use get_module_from_pkg() to map package paths to their containing modules

2. 🚨 ISSUE IDENTIFICATION (FAIL-FAST APPROACH)
   - Use lint(module) to execute golangci-lint on specific modules
   - Review structured JSON reports with file locations, severity, and linter details
   - **CRITICAL**: Stop immediately if errors are found (status: "error")
   - **IMPORTANT**: Fix critical errors before continuing with other modules

3. 🔧 ISSUE RESOLUTION (AUTO-FIX + MANUAL STRATEGY)
   - Address ERRORS first: Go version mismatches, build failures, critical issues
   - NEW: Use fix(module, preview=True) to preview auto-fixable issues
   - NEW: Use fix(module) to automatically fix formatting, imports, comment spacing
   - Then manually fix remaining issues: code duplications, logic errors, test assertions
   - Apply best practices for Go code quality standards

4. ✅ VALIDATION
   - Execute "make test" to run the full test suite after each fix
   - Re-run lint(module) to verify issues are resolved
   - Only proceed to next module when current module is clean
   - Ensure no regressions in code quality or functionality

INCREMENTAL WORKFLOW (STAGED MODULES ONLY):
```
# Target only staged modules for efficient CI/development workflows
staged_modules = find_staged_modules()
if not staged_modules:
    print("No modules staged - skipping lint")
    exit(0)

# Check only staged modules
for module in staged_modules:
    lint(module)
    # ... rest of workflow
```

COMPREHENSIVE WORKFLOW (ALL MODULES):
```
# Discover all modules
modules = find_modules()

# Check modules one by one - STOP ON ERRORS
for module in modules:
    results = lint(module)

    # Parse JSON response
    if results.status == "error":
        print(f"CRITICAL: {module} has errors - STOP AND FIX FIRST")
        # Fix the error (Go version, build issues, etc.)
        # Re-run lint(module) until status != "error"
        break  # Don't continue until fixed

    elif results.status == "completed_with_issues":
        print(f"Issues found in {module}")

        # NEW: Auto-fix simple issues first
        preview = fix(module, preview=True)
        if preview.estimated_fixable_issues > 0:
            print(f"Auto-fixing {preview.estimated_fixable_issues} issues...")
            fix_result = fix(module)

        # Re-lint to see remaining issues
        results = lint(module)

        # Manual fix remaining complex issues
        if results.status == "completed_with_issues":
            print(f"Manual fixes needed for remaining issues")
            # Fix remaining issues, run make test, re-lint
            # Only proceed when clean or acceptable

    else:  # status == "success"
        print(f"{module} is clean ✅")

# Only run make test after ALL critical errors are resolved
```

ERROR HANDLING PRIORITY:
- "error" status: STOP EVERYTHING - fix immediately
- "completed_with_issues": Fix before moving to next module
- "success": Continue to next module

LINTER CONFIG: Uses project's .golangci.yml
OUTPUT FORMAT: Structured JSON with file paths, line numbers, and issue details
"""

# Project root directory (parent of .mcp directory)
PROJECT_ROOT = Path(__file__).parent.parent.absolute()
GOLANGCI_CONFIG = PROJECT_ROOT / ".golangci.yml"


@mcp.tool
def lint(module: str) -> str:
    """
    Lints the provided Go module and returns any issues found.

    Args:
        module (str): The Go module name/directory to be linted (e.g., 'rover', 'identity', 'file-manager').

    Returns:
        str: A structured JSON report of linting issues found in the module.
    """
    try:
        # Resolve module directory
        module_dir = PROJECT_ROOT / module

        if not module_dir.exists():
            return json.dumps({
                "error": f"Module directory '{module}' not found in project root",
                "available_modules": _get_available_modules()
            }, indent=2)

        # Check if go.mod exists in the module directory
        go_mod_path = module_dir / "go.mod"
        if not go_mod_path.exists():
            return json.dumps({
                "error": f"No go.mod found in module directory '{module}'",
                "path": str(module_dir)
            }, indent=2)

        # Execute golangci-lint
        return _execute_golangci_lint(module_dir, module)

    except Exception as e:
        return json.dumps({
            "error": f"Failed to lint module '{module}': {str(e)}"
        }, indent=2)


@mcp.tool
def find_modules() -> list[str]:
    """
    Finds and returns a list of Go module names available for linting.

    Returns:
        list[str]: A list of Go module names.
    """
    return _get_available_modules()


def _get_available_modules() -> List[str]:
    """Get list of available Go modules in the project."""
    modules = []

    # Check for direct modules in project root
    for item in PROJECT_ROOT.iterdir():
        if item.is_dir() and (item / "go.mod").exists():
            modules.append(item.name)

    # Check for nested modules (like api modules)
    for go_mod_path in PROJECT_ROOT.rglob("go.mod"):
        if go_mod_path.parent != PROJECT_ROOT:
            rel_path = go_mod_path.parent.relative_to(PROJECT_ROOT)
            modules.append(str(rel_path))

    return sorted(list(set(modules)))


@mcp.tool
def find_staged_modules() -> list[str]:
    """
    Identifies Go modules that have staged changes based on Git diffs.

    Returns:
        list[str]: A list of Go module names with staged changes.
    """
    try:
        # Get the list of changed files from git
        git_cmd = ["git", "diff", "--name-only", "--cached"]
        result = subprocess.run(
            git_cmd,
            cwd=PROJECT_ROOT,
            capture_output=True,
            text=True,
            timeout=30
        )

        if result.returncode != 0:
            return []

        output = result.stdout.strip()
        if not output:
            return []
        changed_files = output.split('\n')

        changed_modules = set()

        all_modules = _get_available_modules()
        # Sort modules by path depth, descending, to match the most specific path first
        # (e.g., 'a/b' before 'a').
        sorted_modules = sorted(all_modules, key=lambda p: p.count('/'), reverse=True)

        for file_path_str in changed_files:
            if not file_path_str:
                continue

            abs_file_path = (PROJECT_ROOT / file_path_str).resolve()

            # Find the most specific module for the changed file.
            for module_name in sorted_modules:
                module_path = PROJECT_ROOT / module_name
                try:
                    if abs_file_path.is_relative_to(module_path.resolve()):
                        changed_modules.add(module_name)
                        break  # Module found, proceed to the next file
                except ValueError:
                    continue

        return sorted(list(changed_modules))

    except Exception:
        return []

@mcp.tool
def get_module_from_pkg(pkg: str) -> str:
    """
    Determines the Go module that contains the specified package.

    Args:
        pkg (str): The Go package name/path.

    Returns:
        str: The Go module name/directory that contains the package.
    """
    try:
        # Get all modules first
        modules = _get_available_modules()

        # Check if the package path directly matches a module
        if pkg in modules:
            return pkg

        # For package paths like "github.com/telekom/controlplane/<module>/...",
        # extract the module part
        if pkg.startswith("github.com/telekom/controlplane/"):
            pkg_parts = pkg.replace("github.com/telekom/controlplane/", "").split("/")

            # Try progressively longer paths to find matching modules
            for i in range(1, len(pkg_parts) + 1):
                potential_module = "/".join(pkg_parts[:i])
                if potential_module in modules:
                    return potential_module

        # Check if any package exists as a subdirectory within any module
        for module in modules:
            module_dir = PROJECT_ROOT / module
            if module_dir.exists():
                # Check if package path exists within this module
                for potential_path in [pkg, pkg.replace("github.com/telekom/controlplane/", "")]:
                    if (module_dir / potential_path).exists():
                        return module

        return json.dumps({
            "error": f"Package '{pkg}' not found in any module",
            "available_modules": modules
        }, indent=2)

    except Exception as e:
        return json.dumps({
            "error": f"Failed to determine module for package '{pkg}': {str(e)}"
        }, indent=2)

@mcp.tool
def get_info_about_module(module: str) -> str:
    """
    Provides information about the specified Go module.

    Args:
        module (str): The Go module name/directory.

    Returns:
        str: A readable string containing information about the module.
    """
    try:
        # Resolve module directory
        module_dir = PROJECT_ROOT / module

        if not module_dir.exists():
            return json.dumps({
                "error": f"Module directory '{module}' not found",
                "available_modules": _get_available_modules()
            }, indent=2)

        # Collect module information
        module_info = {
            "module": module,
            "path": str(module_dir),
            "go_mod_info": {},
            "readme_content": "",
            "structure": {},
            "has_tests": False
        }

        # Read go.mod file
        go_mod_path = module_dir / "go.mod"
        if go_mod_path.exists():
            try:
                with open(go_mod_path, 'r') as f:
                    go_mod_content = f.read()

                # Extract module name and Go version
                for line in go_mod_content.split('\n'):
                    line = line.strip()
                    if line.startswith('module '):
                        module_info["go_mod_info"]["module_name"] = line.replace('module ', '')
                    elif line.startswith('go '):
                        module_info["go_mod_info"]["go_version"] = line.replace('go ', '')

                # Count dependencies
                require_section = False
                dep_count = 0
                for line in go_mod_content.split('\n'):
                    line = line.strip()
                    if line.startswith('require ('):
                        require_section = True
                        continue
                    elif line == ')' and require_section:
                        require_section = False
                        continue
                    elif require_section and line and not line.startswith('//'):
                        dep_count += 1
                    elif line.startswith('require ') and not line.endswith('('):
                        dep_count += 1

                module_info["go_mod_info"]["dependencies_count"] = dep_count

            except Exception as e:
                module_info["go_mod_info"]["error"] = f"Failed to parse go.mod: {str(e)}"

        # Read README file
        readme_files = ['README.md', 'README.rst', 'README.txt', 'README', 'readme.md', 'readme.txt']
        for readme_name in readme_files:
            readme_path = module_dir / readme_name
            if readme_path.exists():
                try:
                    with open(readme_path, 'r', encoding='utf-8') as f:
                        readme_content = f.read()
                        # Limit README content to first 2000 characters for readability
                        if len(readme_content) > 2000:
                            module_info["readme_content"] = readme_content[:2000] + "\n\n... (truncated)"
                        else:
                            module_info["readme_content"] = readme_content
                    break
                except Exception as e:
                    module_info["readme_content"] = f"Error reading README: {str(e)}"

        # Analyze directory structure
        try:
            dirs = []
            files = []
            test_files = 0

            for item in module_dir.iterdir():
                if item.is_dir():
                    dirs.append(item.name)
                elif item.is_file():
                    files.append(item.name)
                    if item.name.endswith('_test.go'):
                        test_files += 1

            module_info["structure"] = {
                "directories": sorted(dirs),
                "files_count": len(files),
                "test_files_count": test_files
            }
            module_info["has_tests"] = test_files > 0

        except Exception as e:
            module_info["structure"]["error"] = f"Failed to analyze structure: {str(e)}"

        # Format output as readable text
        output_lines = []
        output_lines.append(f"# Module: {module}")
        output_lines.append(f"Path: {module_info['path']}")
        output_lines.append("")

        # Go module information
        if module_info["go_mod_info"]:
            output_lines.append("## Go Module Information")
            if "module_name" in module_info["go_mod_info"]:
                output_lines.append(f"Module Name: {module_info['go_mod_info']['module_name']}")
            if "go_version" in module_info["go_mod_info"]:
                output_lines.append(f"Go Version: {module_info['go_mod_info']['go_version']}")
            if "dependencies_count" in module_info["go_mod_info"]:
                output_lines.append(f"Dependencies: {module_info['go_mod_info']['dependencies_count']}")
            output_lines.append("")

        # Structure information
        if module_info["structure"]:
            output_lines.append("## Structure")
            if "files_count" in module_info["structure"]:
                output_lines.append(f"Files: {module_info['structure']['files_count']}")
            if "test_files_count" in module_info["structure"]:
                output_lines.append(f"Test Files: {module_info['structure']['test_files_count']}")
            if "directories" in module_info["structure"] and module_info["structure"]["directories"]:
                output_lines.append(f"Directories: {', '.join(module_info['structure']['directories'])}")
            output_lines.append(f"Has Tests: {'Yes' if module_info['has_tests'] else 'No'}")
            output_lines.append("")

        # README content
        if module_info["readme_content"]:
            output_lines.append("## README")
            output_lines.append(module_info["readme_content"])
        else:
            output_lines.append("## README")
            output_lines.append("No README file found")

        return "\n".join(output_lines)

    except Exception as e:
        return json.dumps({
            "error": f"Failed to get info for module '{module}': {str(e)}"
        }, indent=2)


@mcp.tool
def fix(module: str, preview: bool = False) -> str:
    """
    Automatically fixes simple linting issues in the provided Go module.

    Fixes formatting, imports, and simple style issues using golangci-lint --fix.
    Complex issues like code duplication and logic errors require manual fixes.

    Args:
        module (str): The Go module name/directory to auto-fix.
        preview (bool): If True, shows current issues without applying fixes.

    Returns:
        str: JSON report of fixes applied and remaining issues.
    """
    try:
        # Resolve module directory
        module_dir = PROJECT_ROOT / module

        if not module_dir.exists():
            return json.dumps({
                "error": f"Module directory '{module}' not found in project root",
                "available_modules": _get_available_modules()
            }, indent=2)

        # Check if go.mod exists
        go_mod_path = module_dir / "go.mod"
        if not go_mod_path.exists():
            return json.dumps({
                "error": f"No go.mod found in module directory '{module}'",
                "path": str(module_dir)
            }, indent=2)

        # Execute fix operation
        return _execute_golangci_fix(module_dir, module, preview)

    except Exception as e:
        return json.dumps({
            "error": f"Failed to fix module '{module}': {str(e)}"
        }, indent=2)


@mcp.tool
def analyze_code_coverage(module: str) -> str:
    """
    Run the Go test coverage analysis for the specified module and return coverage statistics.

    Args:
        module (str): The Go module name/directory to analyze.

    Returns:
        str: A report of code coverage statistics. Which includes total coverage percentage,
             uncovered files, and suggestions for improvement.
    """
    try:
        # Resolve module directory
        module_dir = PROJECT_ROOT / module

        if not module_dir.exists():
            return json.dumps({
                "error": f"Module directory '{module}' not found in project root",
                "available_modules": _get_available_modules()
            }, indent=2)

        # Check if go.mod exists
        go_mod_path = module_dir / "go.mod"
        if not go_mod_path.exists():
            return json.dumps({
                "error": f"No go.mod found in module directory '{module}'",
                "path": str(module_dir)
            }, indent=2)

        # Execute coverage analysis
        return _execute_coverage_analysis(module_dir, module)

    except Exception as e:
        return json.dumps({
            "error": f"Failed to analyze coverage for module '{module}': {str(e)}"
        }, indent=2)


def _execute_coverage_analysis(module_dir: Path, module_name: str) -> str:
    """Execute Go test coverage analysis and return structured results."""
    try:
        coverage_file = module_dir / "coverage.out"

        # First, run tests with coverage
        test_cmd = ["go", "test", f"-coverprofile={coverage_file}", "./..."]
        test_result = subprocess.run(
            test_cmd,
            cwd=module_dir,
            capture_output=True,
            text=True,
            timeout=300
        )

        if test_result.returncode != 0:
            return json.dumps({
                "module": module_name,
                "status": "test_failure",
                "error_message": "Tests failed, cannot generate coverage report.",
                "test_output": (test_result.stderr or test_result.stdout)[:5000]
            }, indent=2)

        if not coverage_file.exists():
            return json.dumps({
                "module": module_name,
                "status": "no_coverage_data",
                "message": "No coverage data generated. Module may not have any tests."
            }, indent=2)

        # Get overall and per-function coverage
        coverage_cmd = ["go", "tool", "cover", f"-func={coverage_file}"]
        coverage_result = subprocess.run(
            coverage_cmd,
            cwd=module_dir,
            capture_output=True,
            text=True,
            timeout=60
        )

        # Clean up coverage file immediately
        try:
            coverage_file.unlink()
        except OSError:
            pass

        if coverage_result.returncode != 0:
            return json.dumps({
                "module": module_name,
                "status": "coverage_analysis_failed",
                "error_message": "Failed to analyze coverage data.",
                "coverage_output": coverage_result.stderr
            }, indent=2)

        coverage_output = coverage_result.stdout
        coverage_summary = _parse_coverage_output(coverage_output)
        file_summary, low_coverage_files = _summarize_file_coverage(coverage_output)

        report = {
            "module": module_name,
            "status": "success",
            "coverage_summary": coverage_summary,
            "file_summary": file_summary,
            "low_coverage_files": low_coverage_files,
            "coverage_analysis": _analyze_coverage_quality(coverage_summary.get("total_coverage", 0.0)),
            "suggestions": _generate_coverage_suggestions(coverage_summary, low_coverage_files)
        }

        return json.dumps(report, indent=2)

    except subprocess.TimeoutExpired:
        return json.dumps({
            "module": module_name,
            "status": "timeout",
            "error_message": "Coverage analysis timed out."
        }, indent=2)
    except Exception as e:
        return json.dumps({
            "module": module_name,
            "status": "error",
            "error_message": f"An unexpected error occurred: {str(e)}"
        }, indent=2)


def _parse_coverage_output(coverage_output: str) -> Dict[str, Any]:
    """Parse go tool cover output to extract overall coverage statistics."""
    lines = coverage_output.strip().split('\n')
    total_line = next((line for line in reversed(lines) if "total:" in line.lower()), None)

    if not total_line:
        return {"total_coverage": 0.0, "error": "Could not find total coverage line."}

    try:
        percentage_str = total_line.split()[-1].rstrip('%')
        total_coverage = float(percentage_str)
        return {
            "total_coverage": round(total_coverage, 2),
            "total_line": total_line.strip()
        }
    except (ValueError, IndexError):
        return {"total_coverage": 0.0, "error": "Could not parse total coverage percentage."}


def _summarize_file_coverage(coverage_output: str, low_coverage_threshold: float = 50.0):
    """
    Parses file-level coverage, summarizes it by file, and identifies low-coverage files.
    """
    lines = coverage_output.strip().split('\n')
    function_coverage = []
    for line in lines:
        if not line or line.startswith("total:"):
            continue
        
        parts = line.strip().split('\t')
        if len(parts) < 2:
            continue
        
        full_func_name = parts[0]
        percentage_str = parts[-1]

        last_colon_index = full_func_name.rfind(':')
        if last_colon_index == -1:
            continue
        
        file_path = full_func_name[:last_colon_index]
        # Clean up file path from any line numbers if they exist
        file_path_parts = file_path.split(':')
        if len(file_path_parts) > 1 and file_path_parts[-1].isdigit():
            file_path = ':'.join(file_path_parts[:-1])

        try:
            coverage_pct = float(percentage_str.rstrip('%'))
            function_coverage.append({"file": file_path, "coverage": coverage_pct, "is_covered": coverage_pct > 0})
        except ValueError:
            continue

    file_stats = {}
    for func in function_coverage:
        if func["file"] not in file_stats:
            file_stats[func["file"]] = {"total": 0, "covered": 0}
        file_stats[func["file"]]["total"] += 1
        if func["is_covered"]:
            file_stats[func["file"]]["covered"] += 1
    
    summary_list = [
        {
            "file": file,
            "coverage": round((data["covered"] / data["total"]) * 100, 2) if data["total"] > 0 else 0.0,
            "functions_total": data["total"],
            "functions_covered": data["covered"],
        }
        for file, data in file_stats.items()
    ]
    summary_list = sorted(summary_list, key=lambda x: x["coverage"])

    low_coverage_files = [f for f in summary_list if f["coverage"] < low_coverage_threshold]

    return summary_list, low_coverage_files


def _analyze_coverage_quality(total_coverage: float) -> Dict[str, Any]:
    """Analyze coverage quality and provide assessment."""
    if total_coverage >= 80:
        quality = "excellent"
        assessment = "Great coverage! Your code is well-tested."
    elif total_coverage >= 70:
        quality = "good"
        assessment = "Good coverage. Consider adding a few more tests for critical paths."
    elif total_coverage >= 50:
        quality = "moderate"
        assessment = "Moderate coverage. Adding more tests would improve code reliability."
    else:
        quality = "low"
        assessment = "Low coverage. Significant testing improvements are needed."

    return {
        "quality_rating": quality,
        "assessment": assessment
    }


def _generate_coverage_suggestions(
    coverage_summary: Dict[str, Any],
    low_coverage_files: List[Dict[str, Any]]
) -> List[str]:
    """Generate suggestions for improving code coverage."""
    suggestions = []
    total_coverage = coverage_summary.get("total_coverage", 0.0)

    if total_coverage < 50:
        suggestions.append("🎯 Priority: Add basic unit tests for core functionality.")
    elif total_coverage < 70:
        suggestions.append("✅ Add tests for error handling and edge cases.")
    else:
        suggestions.append("🏆 Focus on covering remaining complex logic and edge cases.")

    if low_coverage_files:
        suggestions.append(f"⚠️ Focus on improving test coverage for these {len(low_coverage_files)} files:")
        for file_info in low_coverage_files[:3]:  # Suggest up to 3 files
            suggestions.append(f"  - {file_info['file']} ({file_info['coverage']}% coverage)")

    suggestions.append("💡 Use table-driven tests for comprehensive case coverage.")
    suggestions.append("🔄 Run tests with the -race flag to detect data races in concurrent code.")
    return suggestions


def _execute_golangci_fix(module_dir: Path, module_name: str, preview: bool) -> str:
    """Execute golangci-lint with --fix flag and return results."""
    try:
        if preview:
            # Preview mode: just show current issues
            current_issues_result = _execute_golangci_lint(module_dir, module_name)
            try:
                current_data = json.loads(current_issues_result)
                fixable_count = 0
                fixable_types = []

                if "issues" in current_data:
                    for issue in current_data["issues"]:
                        linter = issue.get("linter", "")
                        # Count issues from auto-fixable linters
                        if linter in ["gofmt", "goimports", "revive"]:
                            fixable_count += 1
                            if linter not in fixable_types:
                                fixable_types.append(linter)

                return json.dumps({
                    "module": module_name,
                    "preview_mode": True,
                    "current_total_issues": current_data.get("summary", {}).get("total_issues", 0),
                    "estimated_fixable_issues": fixable_count,
                    "fixable_linters": fixable_types,
                    "message": f"Estimated {fixable_count} issues can be auto-fixed. Use fix('{module_name}') to apply fixes.",
                    "auto_fixable_types": ["formatting (gofmt)", "imports (goimports)", "comment spacing (revive)"],
                    "manual_fix_required": ["code duplication", "logic errors", "test assertions", "unused parameters"]
                }, indent=2)

            except json.JSONDecodeError:
                return json.dumps({
                    "module": module_name,
                    "preview_mode": True,
                    "error": "Could not parse current lint results",
                    "raw_result": current_issues_result
                }, indent=2)

        # Apply fixes mode
        cmd = [
            "golangci-lint",
            "run",
            "--config", str(GOLANGCI_CONFIG),
            "--fix",
            "./..."
        ]

        # Execute fix command
        result = subprocess.run(
            cmd,
            cwd=module_dir,
            capture_output=True,
            text=True,
            timeout=300
        )

        # After fixing, run lint again to see what remains
        remaining_issues_result = _execute_golangci_lint(module_dir, module_name)

        return json.dumps({
            "module": module_name,
            "status": "fix_completed",
            "fix_result": {
                "return_code": result.returncode,
                "stdout": result.stdout,
                "stderr": result.stderr if result.stderr else ""
            },
            "remaining_issues_summary": _extract_summary_from_lint_result(remaining_issues_result),
            "message": f"Auto-fix completed for {module_name}. Run lint('{module_name}') for detailed remaining issues."
        }, indent=2)

    except subprocess.TimeoutExpired:
        return json.dumps({
            "module": module_name,
            "status": "timeout",
            "error_message": "Fix process timed out after 5 minutes"
        }, indent=2)
    except Exception as e:
        return json.dumps({
            "module": module_name,
            "status": "error",
            "error_message": str(e)
        }, indent=2)


def _extract_summary_from_lint_result(lint_result: str) -> Dict[str, Any]:
    """Extract summary information from lint result JSON."""
    try:
        data = json.loads(lint_result)
        if "summary" in data:
            return data["summary"]
        else:
            return {"message": "No issues found"}
    except json.JSONDecodeError:
        return {"error": "Could not parse lint results"}


def _execute_golangci_lint(module_dir: Path, module_name: str) -> str:
    """Execute golangci-lint and return structured results."""
    try:
        # Build golangci-lint command
        cmd = [
            "golangci-lint",
            "run",
            "--config", str(GOLANGCI_CONFIG),
            "--output.json.path", "stdout",
            "./..."
        ]

        # Execute in module directory
        result = subprocess.run(
            cmd,
            cwd=module_dir,
            capture_output=True,
            text=True,
            timeout=300  # 5 minute timeout
        )

        # Parse results
        if result.returncode == 0:
            # No issues found
            return json.dumps({
                "module": module_name,
                "status": "success",
                "issues": [],
                "summary": {
                    "total_issues": 0,
                    "severity_breakdown": {},
                    "linter_breakdown": {}
                }
            }, indent=2)

        # Parse JSON output from golangci-lint
        if result.stdout:
            try:
                lint_data = json.loads(result.stdout)
                return _format_lint_results(module_name, lint_data)
            except json.JSONDecodeError:
                # Fallback to text output if JSON parsing fails
                return json.dumps({
                    "module": module_name,
                    "status": "completed_with_issues",
                    "raw_output": result.stdout,
                    "stderr": result.stderr if result.stderr else ""
                }, indent=2)

        # Handle stderr or other errors
        return json.dumps({
            "module": module_name,
            "status": "error",
            "error_message": result.stderr or "Unknown error occurred",
            "return_code": result.returncode
        }, indent=2)

    except subprocess.TimeoutExpired:
        return json.dumps({
            "module": module_name,
            "status": "timeout",
            "error_message": "Linting process timed out after 5 minutes"
        }, indent=2)
    except FileNotFoundError:
        return json.dumps({
            "module": module_name,
            "status": "error",
            "error_message": "golangci-lint not found. Please ensure it's installed and in PATH."
        }, indent=2)


def _format_lint_results(module_name: str, lint_data: Dict[str, Any]) -> str:
    """Format golangci-lint JSON results into structured output."""
    issues = lint_data.get("Issues", [])

    # Group issues by severity and linter
    severity_breakdown = {}
    linter_breakdown = {}

    formatted_issues = []
    for issue in issues:
        severity = issue.get("Severity", "unknown")
        linter = issue.get("FromLinter", "unknown")

        # Count by severity
        severity_breakdown[severity] = severity_breakdown.get(severity, 0) + 1

        # Count by linter
        linter_breakdown[linter] = linter_breakdown.get(linter, 0) + 1

        # Format issue
        formatted_issue = {
            "file": issue.get("Pos", {}).get("Filename", "unknown"),
            "line": issue.get("Pos", {}).get("Line", 0),
            "column": issue.get("Pos", {}).get("Column", 0),
            "severity": severity,
            "message": issue.get("Text", ""),
            "linter": linter,
            "rule": issue.get("Rule", "")
        }
        formatted_issues.append(formatted_issue)

    return json.dumps({
        "module": module_name,
        "status": "completed_with_issues" if issues else "success",
        "issues": formatted_issues,
        "summary": {
            "total_issues": len(issues),
            "severity_breakdown": severity_breakdown,
            "linter_breakdown": linter_breakdown
        }
    }, indent=2)


if __name__ == "__main__":
    mcp.run(transport="stdio")