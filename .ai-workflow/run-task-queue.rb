#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"
require "shellwords"

def run!(*cmd)
  puts "+ #{cmd.shelljoin}"
  ok = system(*cmd)
  raise "Command failed: #{cmd.shelljoin}" unless ok
end

def run_with_env!(env, *cmd)
  env_preview = env.map { |key, value| "#{key}=#{value.to_s.shellescape}" }.join(" ")
  puts "+ #{env_preview} #{cmd.shelljoin}"
  ok = system(env, *cmd)
  raise "Command failed: #{env_preview} #{cmd.shelljoin}" unless ok
end

def capture!(*cmd)
  out, err, status = Open3.capture3(*cmd)
  raise "Command failed: #{cmd.shelljoin}\n#{err}" unless status.success?

  out.strip
end

def quiet_success?(*cmd)
  File.open(File::NULL, "w") do |null|
    system(*cmd, out: null, err: null)
  end
end

def remote_branch_exists?(branch)
  quiet_success?("git", "ls-remote", "--exit-code", "--heads", "origin", branch)
end

def local_ref_exists?(ref)
  quiet_success?("git", "rev-parse", "--verify", "--quiet", ref)
end

def valid_branch?(branch)
  quiet_success?("git", "check-ref-format", "--branch", branch)
end

def fetch_remote_branch!(branch)
  run!("git", "fetch", "origin", "+refs/heads/#{branch}:refs/remotes/origin/#{branch}")
  "origin/#{branch}"
end

def resolve_base_ref!(base)
  return fetch_remote_branch!(base) if remote_branch_exists?(base)
  return base if local_ref_exists?(base)

  raise "Base branch/ref not found: #{base}"
end

def append_summary(lines)
  summary_path = ENV["GITHUB_STEP_SUMMARY"]
  return if summary_path.to_s.empty?

  File.open(summary_path, "a") do |file|
    file.puts(lines.join("\n"))
    file.puts
  end
end

def markdown_cell(value)
  value.to_s.gsub("|", "\\|").gsub("\n", "<br>")
end

def mermaid_node_id(id)
  "task_#{id.to_s.gsub(/[^A-Za-z0-9_]/, "_")}"
end

def mermaid_label(value)
  value.to_s.gsub("\\", "\\\\\\").gsub("\"", "\\\"").gsub("\n", "<br/>")
end

repo_root = capture!("git", "rev-parse", "--show-toplevel")
Dir.chdir(repo_root)

queue_path = ARGV[0] || ".ai-workflow/queues/employee-management.json"
queue = JSON.parse(File.read(queue_path))
tasks = queue.fetch("tasks")

start_from = ENV.fetch("START_FROM", "").strip
max_tasks = Integer(ENV.fetch("MAX_TASKS", "1"))
dry_run = ENV.fetch("DRY_RUN", "") == "1"
skip_existing = ENV.fetch("SKIP_EXISTING_BRANCHES", queue.fetch("skip_existing_branches", true).to_s) != "false"

started = start_from.empty?
processed = 0
results = []

puts "Queue: #{queue.fetch("name", queue_path)}"
puts "Start from: #{start_from.empty? ? "(first enabled task)" : start_from}"
puts "Max tasks: #{max_tasks.zero? ? "all" : max_tasks}"
puts "Dry run: #{dry_run}"
puts "Skip existing branches: #{skip_existing}"

tasks.each do |task|
  id = task.fetch("id")
  next unless task.fetch("enabled", true)

  unless started
    started = id == start_from
    next unless started
  end

  break if max_tasks.positive? && processed >= max_tasks

  title = task.fetch("title")
  branch = task.fetch("branch")
  base = task.fetch("base", "main")
  prompt = task.fetch("task")

  raise "Invalid branch for #{id}: #{branch}" unless valid_branch?(branch)

  if skip_existing && remote_branch_exists?(branch)
    puts "Skipping #{id}: remote branch exists (#{branch})"
    results << [id, title, branch, "skipped", "remote branch exists"]
    next
  end

  puts
  puts "== #{id}: #{title} =="
  puts "Branch: #{branch}"
  puts "Base: #{base}"

  if dry_run
    results << [id, title, branch, "dry-run", "would run from #{base}"]
    processed += 1
    next
  end

  base_ref = resolve_base_ref!(base)
  run!("git", "switch", "-C", branch, base_ref)

  run_with_env!({ "CODEX_REVIEW_BASE_REF" => base_ref }, ".ai-workflow/auto-codex-cycle.sh", prompt)

  if capture!("git", "status", "--porcelain").empty?
    puts "No changes for #{id}."
    results << [id, title, branch, "no changes", ""]
  else
    run!("git", "config", "user.name", "zxygzhong-art-ai")
    run!("git", "config", "user.email", "zxygzhong-art-ai@users.noreply.github.com")
    run!("git", "add", "-A")
    run!("git", "commit", "-m", "chore(ai): #{id} #{title}")
    commit = capture!("git", "rev-parse", "--short", "HEAD")
    run!("git", "push", "-u", "origin", "HEAD:#{branch}")
    results << [id, title, branch, "pushed", commit]
  end

  processed += 1
end

summary = []
summary << "## Local Codex Queue"
summary << ""
summary << "- Queue: `#{queue.fetch("name", queue_path)}`"
summary << "- Dry run: `#{dry_run}`"
summary << "- Max tasks: `#{max_tasks.zero? ? "all" : max_tasks}`"
summary << ""
summary << "### Queue Graph"
summary << ""
summary << "```mermaid"
summary << "flowchart LR"
enabled_tasks = tasks.select { |task| task.fetch("enabled", true) }
result_by_id = results.to_h { |id, title, branch, result, detail| [id, [title, branch, result, detail]] }
enabled_tasks.each do |task|
  id = task.fetch("id")
  title = task.fetch("title")
  branch = task.fetch("branch")
  result = result_by_id.fetch(id, [nil, nil, task.fetch("status", "queued"), nil])[2]
  label = "#{id}<br/>#{title}<br/>#{branch}<br/>#{result}"
  summary << "  #{mermaid_node_id(id)}[\"#{mermaid_label(label)}\"]"
end
enabled_tasks.each_cons(2) do |left, right|
  summary << "  #{mermaid_node_id(left.fetch("id"))} --> #{mermaid_node_id(right.fetch("id"))}"
end
summary << "```"
summary << ""
summary << "| ID | Task | Branch | Result | Detail |"
summary << "| --- | --- | --- | --- | --- |"
results.each do |id, title, branch, result, detail|
  summary << "| `#{markdown_cell(id)}` | #{markdown_cell(title)} | `#{markdown_cell(branch)}` | #{markdown_cell(result)} | #{markdown_cell(detail)} |"
end
summary << "| | | | no matching tasks | |" if results.empty?
summary << ""

append_summary(summary)

puts
puts summary.join("\n")
