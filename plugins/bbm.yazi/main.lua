--- @since 26.5.6

local M = {}
local lib = require(".lib")
local browse_pkg = require(".browse")
local pull = require(".pull")

local Browse = browse_pkg.M
local BROWSE_LOADING = browse_pkg.LOADING
local BUCKETS_LOADING = browse_pkg.BUCKETS_LOADING
local BROWSE_BUCKETS = "__buckets__"

-- Modal UI lives on M (returned plugin table), same as mount.yazi.
-- Yazi's Modal looks up :redraw/:layout via raw fields on the host table.
M.keys = Browse.keys
M.permit = Browse.permit
M.fetch_ls = Browse.fetch_ls
M.fetch_buckets = Browse.fetch_buckets
M.parse_ls = Browse.parse_ls
for name, fn in pairs(Browse) do
	if type(fn) == "function" and name ~= "fetch_ls" and name ~= "parse_ls" then
		M[name] = fn
	end
end

M.notify = function(title, content, level)
	ya.notify({
		title = title or "bbm",
		content = content or "",
		timeout = 8,
		level = level or "info",
	})
end

local apply_config = ya.sync(function(st, opts)
	if type(opts) ~= "table" then
		return
	end

	if opts.prefix then
		st.prefix = opts.prefix
	end
	if opts.browse_prefix then
		st.browse_prefix = opts.browse_prefix
	elseif opts.prefix then
		st.browse_prefix = opts.prefix
	end
	if opts.notify_title then
		st.notify_title = opts.notify_title
	end
	if opts.bbm_bin then
		st.bbm_bin = opts.bbm_bin
	end
	if opts.rclone_bin then
		st.rclone_bin = opts.rclone_bin
	end
	if opts.default_bucket then
		st.default_bucket = opts.default_bucket
	end
	if opts.mount then
		st.mount = opts.mount
	end

	st.prefix = st.prefix or ""
	st.browse_prefix = st.browse_prefix or st.prefix
	st.notify_title = st.notify_title or "bbm"

	lib.log(
		"setup: mount="
			.. tostring(st.mount ~= nil)
			.. " path="
			.. tostring(st.mount and st.mount.path)
	)
end)

local get_config = ya.sync(function(st)
	return {
		prefix = st.prefix or "",
		browse_prefix = st.browse_prefix or st.prefix or "",
		notify_title = st.notify_title or "bbm",
		bbm_bin = st.bbm_bin,
		rclone_bin = st.rclone_bin,
		mount = st.mount,
		default_bucket = st.default_bucket,
	}
end)

local function cd_to(dest)
	lib.log("cd_to: " .. dest)
	ya.emit("cd", { dest, raw = true })
	lib.log("cd_to: emitted")
end

local function plugin_state(st)
	return {
		prefix = st.prefix or "",
		browse_prefix = st.browse_prefix or st.prefix or "",
		notify_title = st.notify_title or "bbm",
		bbm_bin = st.bbm_bin,
		rclone_bin = st.rclone_bin,
		mount = st.mount,
		default_bucket = st.default_bucket,
		notify = M.notify,
	}
end

-- Browse modal sync blocks: read/write UI on M, not plugin config state.
local toggle_ui = ya.sync(function(_st)
	lib.log("toggle_ui: children=" .. tostring(M.children ~= nil))
	if M.children then
		Modal:children_remove(M.children)
		M.children = nil
	else
		M.children = Modal:children_add(M, 10)
		lib.log("toggle_ui: _area=" .. tostring(M._area ~= nil))
	end
	ui.render()
end)

local update_entries = ya.sync(function(_st, entries)
	M.entries = entries
	M.cursor = math.max(0, math.min(M.cursor or 0, math.max(0, #entries - 1)))
	ui.render()
end)

local update_prefix = ya.sync(function(_st, prefix)
	M.prefix = prefix
	ui.render()
end)

local update_cursor = ya.sync(function(_st, delta)
	if #(M.entries or {}) == 0 then
		M.cursor = 0
	else
		M.cursor = ya.clamp(0, (M.cursor or 0) + delta, #M.entries - 1)
	end
	ui.render()
end)

local active_entry = ya.sync(function(_st)
	return M.entries[(M.cursor or 0) + 1]
end)

local get_cwd = ya.sync(function()
	return tostring(cx.active.current.cwd)
end)

local update_mode = ya.sync(function(_st, mode)
	M.mode = mode
	ui.render()
end)

local update_bucket_name = ya.sync(function(_st, bucket)
	M.bucket = bucket
	ui.render()
end)

local function browse_request_buckets(fetch_tx)
	lib.log("request_buckets")
	update_mode("buckets")
	update_prefix("")
	update_entries(BUCKETS_LOADING)
	fetch_tx:send(BROWSE_BUCKETS)
end

local function browse_request_fetch(prefix, fetch_tx)
	lib.log("request_fetch: " .. tostring(prefix))
	update_prefix(prefix)
	update_entries(BROWSE_LOADING)
	fetch_tx:send(prefix)
end

local function browse_entry(st, job)
	local QUIT = "__quit__"
	lib.log("browse: start args=" .. tostring(job.args[1]) .. "/" .. tostring(job.args[2]))

	M.bbm_bin = st.bbm_bin
	M.default_bucket = st.default_bucket

	if job.args[2] == "refresh" then
		if M.mode == "buckets" then
			local entries, err = M.fetch_buckets(M)
			if err then
				update_entries({ { name = "(" .. err .. ")", kind = "error", key = "" } })
			else
				update_entries(entries)
			end
		else
			local entries, err = M.fetch_ls(M, M.prefix or "")
			if err then
				update_entries({ { name = "(" .. err .. ")", kind = "error", key = "" } })
			else
				update_entries(entries)
			end
		end
		return
	end

	M.mode = "buckets"
	M.bucket = st.default_bucket
	M.prefix = ""
	M.entries = BUCKETS_LOADING
	M.cursor = 0

	toggle_ui()
	update_entries(BUCKETS_LOADING)

	local fetch_tx, fetch_rx = ya.chan("mpsc")
	local tx1, rx1 = ya.chan("mpsc")
	local tx2, rx2 = ya.chan("mpsc")
	local cfg = plugin_state(st)

	local function loader()
		lib.log("loader: started")
		while true do
			local prefix = fetch_rx:recv()
			if prefix == QUIT then
				lib.log("loader: quit")
				break
			end
			lib.log("loader: got request prefix=" .. tostring(prefix))
			if prefix == BROWSE_BUCKETS then
				local entries, err = M.fetch_buckets(M)
				if err then
					cfg.notify(cfg.notify_title, "bbm bucket list: " .. err, "error")
					update_entries({ { name = "(" .. err .. ")", kind = "error", key = "" } })
				else
					update_mode("buckets")
					lib.log("loader: buckets=" .. tostring(#entries))
					update_entries(entries)
				end
			else
				local entries, err = M.fetch_ls(M, prefix)
				if err then
					cfg.notify(cfg.notify_title, "bbm ls: " .. err, "error")
					update_entries({ { name = "(" .. err .. ")", kind = "error", key = "" } })
				else
					update_mode("objects")
					lib.log("loader: entries=" .. tostring(#entries))
					update_entries(entries)
				end
			end
		end
	end

	local function producer()
		lib.log("producer: started")
		M.permit[1]:send(true)
		while true do
			M.permit[2]:recv()
			local idx = ya.which({ cands = M.keys, silent = true })
			M.permit[1]:send(true)

			local cand = M.keys[idx] or { run = "noop" }
			local run = cand.run
			if type(run) == "table" then
				run = run[1]
			end
			lib.log("producer: key run=" .. tostring(run))
			tx1:send(run)
			if run == "quit" then
				fetch_tx:send(QUIT)
				toggle_ui()
				lib.log("producer: quit")
				return
			end
		end
	end

	local function consumer1()
		repeat
			local run = rx1:recv()
			if run == "quit" then
				tx2:send(run)
				break
			elseif run == "up" then
				update_cursor(-1)
			elseif run == "down" then
				update_cursor(1)
			elseif run == "back" then
				if M.mode == "buckets" then
					-- already at bucket list
				else
					local p = M.prefix or ""
					if p == "" then
						browse_request_buckets(fetch_tx)
					else
						local parent = p:match("^(.*/)[^/]+/$") or ""
						browse_request_fetch(parent, fetch_tx)
					end
				end
			elseif run == "enter" then
				local entry = active_entry()
				if M.mode == "buckets" and entry and entry.kind == "bucket" then
					update_bucket_name(entry.name)
					local start_prefix = st.browse_prefix or st.prefix or ""
					browse_request_fetch(start_prefix, fetch_tx)
				elseif entry and entry.kind == "dir" then
					browse_request_fetch(entry.key, fetch_tx)
				end
			elseif run == "buckets" then
				browse_request_buckets(fetch_tx)
			else
				tx2:send(run)
			end
		until not run
	end

	local function consumer2()
		repeat
			local run = rx2:recv()
			if run == "quit" then
				break
			elseif run == "refresh" then
				if M.mode == "buckets" then
					browse_request_buckets(fetch_tx)
				else
					browse_request_fetch(M.prefix or "", fetch_tx)
				end
			elseif run == "pull" or run == "pull_decrypt" then
				local entry = active_entry()
				if not entry or entry.kind ~= "file" then
					cfg.notify(cfg.notify_title, "Select a file to pull", "warn")
				else
					local cwd = get_cwd()
					local decrypt = run == "pull_decrypt"
					cfg.bucket = M.bucket
					lib.log("pull: bucket=" .. tostring(M.bucket) .. " key=" .. entry.key .. " decrypt=" .. tostring(decrypt))
					local err = pull.pull_key(cfg, entry.key, cwd, decrypt)
					if err then
						cfg.notify(cfg.notify_title, err, "error")
					elseif decrypt and entry.key:sub(-4) == ".gpg" then
						cfg.notify(cfg.notify_title, "Pulled and decrypted " .. entry.name, "info")
					else
						cfg.notify(cfg.notify_title, "Pulled " .. entry.name, "info")
					end
				end
			end
		until not run
	end

	fetch_tx:send(BROWSE_BUCKETS)
	lib.log("browse: joining loader/producer/consumers")
	ya.join(loader, producer, consumer1, consumer2)
	lib.log("browse: finished")
end

function M.setup(_, opts)
	apply_config(opts or {})
end

function M.entry(st, job)
	local action = job.args[1] or "browse"
	lib.log("main.entry: action=" .. tostring(action))

	if action == "browse" then
		local ok, err = pcall(function()
			browse_entry(st, job)
		end)
		if not ok then
			lib.log_error("browse", err)
			M.notify(st.notify_title, "Browse failed: " .. tostring(err), "error")
		end
		return
	end

	local cfg = get_config()
	local state = {
		prefix = cfg.prefix,
		browse_prefix = cfg.browse_prefix,
		notify_title = cfg.notify_title,
		bbm_bin = cfg.bbm_bin,
		rclone_bin = cfg.rclone_bin,
		mount = cfg.mount,
		default_bucket = cfg.default_bucket,
		notify = M.notify,
	}
	lib.log("main.entry: mount=" .. tostring(cfg.mount ~= nil))

	if action == "push" then
		require(".push").run(state, job)
	elseif action == "pull" then
		require(".pull").run(state, job)
	elseif action == "mount" then
		local dest = require(".mount").run(state, job)
		if dest then
			cd_to(dest)
			M.notify(cfg.notify_title, "Opened " .. dest, "info")
		else
			lib.log("mount: no destination returned")
		end
	else
		M.notify(cfg.notify_title, "Unknown action: " .. action, "error")
	end
end

return M
