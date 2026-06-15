local M = {}
local lib = require(".lib")

local function basename(path)
	return path:match("([^/]+)$") or path
end

local function is_dir(path)
	local status, err = Command("test"):arg("-d"):arg(path):status()
	if not status then
		return false, err
	end
	return status.success
end

local get_targets = ya.sync(function()
	local targets = {}
	for _, url in pairs(cx.active.selected) do
		targets[#targets + 1] = tostring(url)
	end
	if #targets == 0 then
		local h = cx.active.current.hovered
		if h then
			targets[1] = tostring(h.url)
		end
	end
	return targets
end)

local function ask_prefix(state, default)
	local value, event = ya.input({
		pos = { "center", w = 50 },
		title = "B2 key prefix (e.g. bu/my-dir.tar.gz):",
		value = default,
	})
	if event ~= 1 then
		return nil
	end
	if value == "" then
		return default
	end
	return value
end

function M.run(state, job)
	local targets = get_targets()
	if #targets == 0 then
		state.notify(state.notify_title, "Nothing selected", "warn")
		return
	end

	local ok, failed = 0, 0
	for _, path in ipairs(targets) do
		local name = basename(path)
		local default_key = state.prefix .. name
		if is_dir(path) then
			default_key = state.prefix .. name .. ".tar.gz"
		end

		local key = job.args.key or job.args[2]
		if not key then
			key = default_key
			if #targets == 1 then
				local asked = ask_prefix(state, default_key)
				if not asked then
					return
				end
				key = asked
			end
		end

		local err = M.push_one(state, path, key)
		if err then
			failed = failed + 1
			state.notify(state.notify_title, err, "error")
		else
			ok = ok + 1
		end
	end

	if ok > 0 then
		state.notify(state.notify_title, string.format("Pushed %d item(s) to B2", ok), "info")
	end
	if failed > 0 and ok == 0 then
		state.notify(state.notify_title, string.format("%d upload(s) failed", failed), "error")
	end
end

function M.push_one(state, path, key)
	if is_dir(path) then
		return M.push_dir(state, path, key)
	end
	return M.push_file(state, path, key)
end

function M.push_file(state, path, key)
	local output, err = lib.bbm_cmd(state, { "push", "--encrypt", path, key })
		:stdout(Command.PIPED)
		:stderr(Command.PIPED)
		:output()
	if not output then
		return "bbm push failed: " .. tostring(err)
	end
	if not output.status.success then
		local msg = (output.stderr or "") .. (output.stdout or "")
		if msg == "" then
			msg = "exit " .. tostring(output.status.code)
		end
		return "bbm push " .. basename(path) .. ": " .. msg
	end
	return nil
end

function M.push_dir(state, path, key)
	local parent = path:match("^(.*)/[^/]+$") or "."
	local name = basename(path)
	local tarball = os.getenv("TMPDIR") or "/tmp"
	tarball = tarball:gsub("/$", "") .. "/bbm-" .. name .. "-" .. os.time() .. ".tar.gz"

	local tar_out, tar_err = Command("tar")
		:arg({ "-czf", tarball, "-C", parent, name })
		:stdout(Command.PIPED)
		:stderr(Command.PIPED)
		:output()
	if not tar_out or not tar_out.status.success then
		local msg = tar_out and tar_out.stderr or tostring(tar_err)
		return "tar " .. name .. ": " .. (msg or "failed")
	end

	local push_err = M.push_file(state, tarball, key)
	Command("rm"):arg("-f"):arg(tarball):status()
	return push_err
end

return M
