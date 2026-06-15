local M = {}
local lib = require(".lib")

local LABEL = "com.j4y.rclone-j4y-bu"

local function expand(path)
	if path:sub(1, 1) == "~" then
		return (os.getenv("HOME") or "") .. path:sub(2)
	end
	return path
end

local function dest_path(cfg)
	local root = expand(cfg.path):gsub("/$", "")
	local sub = (cfg.start_in or "bu"):gsub("^/", "")
	return root .. "/" .. sub
end

local function mount_alive(path)
	local status = Command("test"):arg("-e"):arg(path):stdout(Command.PIPED):stderr(Command.PIPED):status()
	if not status then
		return false
	end
	return status.success
end

local function kickstart_service()
	local out = Command("id"):arg("-u"):stdout(Command.PIPED):stderr(Command.PIPED):output()
	local uid = out and (out.stdout or ""):gsub("%s+", "") or ""
	if uid == "" then
		return false
	end
	Command("launchctl"):arg({ "kickstart", "-k", "gui/" .. uid .. "/" .. LABEL }):status()
	return true
end

function M.run(state, job)
	local ok, result = pcall(function()
		return M._run(state, job)
	end)
	if not ok then
		lib.log_error("mount", result)
		ya.notify({
			title = "bbm",
			content = "Mount failed: " .. tostring(result),
			timeout = 8,
			level = "error",
		})
		return nil
	end
	return result
end

function M._run(state, job)
	local cfg = state.mount
	lib.log("mount: cfg=" .. tostring(cfg ~= nil) .. " path=" .. tostring(cfg and cfg.path))
	if not cfg or not cfg.path then
		lib.log_error("mount", "mount.path not configured")
		state.notify("bbm", "mount.path not configured — restart Yazi after updating init.lua", "warn")
		return
	end

	local root = expand(cfg.path)
	local dest = dest_path(cfg)
	lib.log("mount: root=" .. root .. " dest=" .. dest)

	if not mount_alive(root) then
		lib.log("mount: not alive, kickstarting launchd")
		state.notify(state.notify_title, "Starting B2 mount…", "info")
		kickstart_service()
	end

	for i = 1, 25 do
		if mount_alive(root) then
			break
		end
		ya.sleep(0.2)
	end

	if not mount_alive(dest) and mount_alive(root) then
		-- bu/ may still be listing; try root first
		dest = root
	end

	if not mount_alive(dest) then
		lib.log("mount: still not readable")
		state.notify(
			state.notify_title,
			"B2 not mounted. Run: bash ~/.config/yazi/plugins/bbm.yazi/install-b2-mount-service.sh",
			"error"
		)
		return
	end

	return dest
end

return M
