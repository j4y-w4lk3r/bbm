local M = {}

function M.bbm_bin(state)
	if state and state.bbm_bin then
		return state.bbm_bin
	end
	for _, path in ipairs({ "/opt/homebrew/bin/bbm", "/usr/local/bin/bbm", "bbm" }) do
		if path == "bbm" then
			return path
		end
		local f = io.open(path, "r")
		if f then
			f:close()
			return path
		end
	end
	return "bbm"
end

function M.tool_path(name)
	for _, path in ipairs({
		"/opt/homebrew/bin/" .. name,
		"/usr/local/bin/" .. name,
		name,
	}) do
		if path == name then
			return path
		end
		local f = io.open(path, "r")
		if f then
			f:close()
			return path
		end
	end
	return name
end

function M.path_env()
	local parts = {
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		os.getenv("PATH") or "",
	}
	local seen, out = {}, {}
	for _, p in ipairs(parts) do
		if p ~= "" and not seen[p] then
			seen[p] = true
			out[#out + 1] = p
		end
	end
	return table.concat(out, ":")
end

function M.log(msg)
	local line = os.date("%Y-%m-%d %H:%M:%S ") .. msg
	ya.dbg(line)
	local home = os.getenv("HOME") or ""
	local f = io.open(home .. "/.local/state/yazi/bbm-browse.log", "a")
	if f then
		f:write(line .. "\n")
		f:close()
	end
end

function M.log_error(ctx, err)
	local line = os.date("%Y-%m-%d %H:%M:%S ") .. ctx .. ": error " .. tostring(err)
	ya.dbg(line)
	local home = os.getenv("HOME") or ""
	local f = io.open(home .. "/.local/state/yazi/bbm-browse.log", "a")
	if f then
		f:write(line .. "\n")
		f:close()
	end
end

function M.bbm_cmd(state, args)
	return Command(M.bbm_bin(state))
		:arg(args)
		:env("PATH", M.path_env())
		:env("HOME", os.getenv("HOME") or "")
end

return M
