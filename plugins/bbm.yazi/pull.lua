local M = {}
local lib = require(".lib")

local function basename(path)
	return path:match("([^/]+)$") or path
end

local function without_gpg(name)
	if name:sub(-4) == ".gpg" then
		return name:sub(1, -5)
	end
	return name
end

local function is_tarball(name)
	return name:sub(-7) == ".tar.gz" or name:sub(-4) == ".tgz"
end

local get_context = ya.sync(function()
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
	return targets, tostring(cx.active.current.cwd)
end)

function M.run(state, job)
	local key = job.args.key or job.args[2]
	if key then
		local cwd = select(2, get_context())
		local decrypt = job.args.decrypt ~= false
		local err = M.pull_key(state, key, cwd, decrypt)
		if err then
			state.notify(state.notify_title, err, "error")
		else
			state.notify(state.notify_title, "Pulled and decrypted " .. basename(key), "info")
		end
		return
	end

	local targets, cwd = get_context()
	if #targets == 0 then
		state.notify(state.notify_title, "Nothing selected", "warn")
		return
	end

	local ok, failed = 0, 0
	for _, path in ipairs(targets) do
		local err
		if path:sub(-4) == ".gpg" then
			err = M.decrypt_local(state, path, cwd)
		else
			state.notify(state.notify_title, "Select a .gpg file or use from B2 browser", "warn")
			return
		end
		if err then
			failed = failed + 1
			state.notify(state.notify_title, err, "error")
		else
			ok = ok + 1
		end
	end

	if ok > 0 then
		state.notify(state.notify_title, string.format("Decrypted %d item(s)", ok), "info")
	end
end

function M.pull_key(state, key, dest_dir, decrypt)
	local name = basename(key)
	local dest = dest_dir:gsub("/$", "") .. "/" .. name

	local pull_out, pull_err = lib.bbm_cmd(state, { "pull", key, dest })
		:stdout(Command.PIPED)
		:stderr(Command.PIPED)
		:output()
	if not pull_out then
		return "bbm pull failed: " .. tostring(pull_err)
	end
	if not pull_out.status.success then
		local msg = pull_out.stderr or "exit " .. tostring(pull_out.status.code)
		return "bbm pull " .. key .. ": " .. msg
	end

	if decrypt ~= false and dest:sub(-4) == ".gpg" then
		return M.decrypt_local(state, dest, dest_dir)
	end
	return nil
end

function M.decrypt_local(state, gpg_path, dest_dir)
	local plain_name = without_gpg(basename(gpg_path))
	local plain_path = dest_dir:gsub("/$", "") .. "/" .. plain_name

	local dec_out, dec_err = Command("ykw")
		:arg({ "decrypt", gpg_path, plain_path })
		:stdout(Command.PIPED)
		:stderr(Command.PIPED)
		:output()
	if not dec_out then
		return "ykw decrypt failed: " .. tostring(dec_err)
	end
	if not dec_out.status.success then
		local msg = dec_out.stderr or "exit " .. tostring(dec_out.status.code)
		return "ykw decrypt " .. basename(gpg_path) .. ": " .. msg
	end

	if is_tarball(plain_name) then
		local extract_dir = dest_dir:gsub("/$", "") .. "/" .. plain_name:gsub("%.tar%.gz$", ""):gsub("%.tgz$", "")
		Command("mkdir"):arg("-p"):arg(extract_dir):status()
		local tar_out, tar_err = Command("tar")
			:arg({ "-xzf", plain_path, "-C", extract_dir })
			:stdout(Command.PIPED)
			:stderr(Command.PIPED)
			:output()
		if not tar_out or not tar_out.status.success then
			local msg = tar_out and tar_out.stderr or tostring(tar_err)
			return "tar extract " .. plain_name .. ": " .. msg
		end
	end

	return nil
end

return M
