/**
 * MessageViewer - LLM API message viewer component
 * Parses Anthropic API request/response JSON into a structured chat view
 * with search, filtering, and raw JSON fallback.
 */
window.VueComponents = window.VueComponents || {};

(function () {
  "use strict";

  var ref = Vue.ref;
  var computed = Vue.computed;
  var watch = Vue.watch;
  var nextTick = Vue.nextTick;

  // === Parsing helpers ===

  function parseSystem(sys) {
    if (!sys) return null;
    if (typeof sys === "string") return sys;
    if (Array.isArray(sys)) {
      return sys.map(function (b) { return b.text || ""; }).join("\n");
    }
    return JSON.stringify(sys);
  }

  function normalizeContent(content) {
    if (typeof content === "string") {
      return [{ type: "text", text: content }];
    }
    if (Array.isArray(content)) return content;
    return [{ type: "text", text: String(content || "") }];
  }

  function parseMessage(msg) {
    return {
      role: msg.role || "unknown",
      parts: normalizeContent(msg.content),
    };
  }

  function parseRequest(jsonStr) {
    if (!jsonStr) return null;
    try {
      var parsed = JSON.parse(jsonStr);
      if (!parsed || typeof parsed !== "object") return { _raw: jsonStr, _parseError: true };
      return {
        model: parsed.model || "",
        maxTokens: parsed.max_tokens || 0,
        stream: !!parsed.stream,
        temperature: parsed.temperature,
        system: parseSystem(parsed.system),
        messages: (parsed.messages || []).map(parseMessage),
        tools: parsed.tools || [],
        thinking: parsed.thinking || null,
        _raw: jsonStr,
      };
    } catch (e) {
      return { _raw: jsonStr, _parseError: true };
    }
  }

  function parseResponse(jsonStr) {
    if (!jsonStr) return null;
    try {
      var parsed = JSON.parse(jsonStr);
      if (!parsed || typeof parsed !== "object") return { _raw: jsonStr, _parseError: true };
      // Handle Anthropic error response format: {"type":"error","error":{"type":"...","message":"..."}}
      if (parsed.type === "error" && parsed.error) {
        var errMsg = parsed.error.message || JSON.stringify(parsed.error);
        return {
          content: [{ type: "text", text: errMsg }],
          _isError: true,
          errorType: parsed.error.type || "unknown_error",
          _raw: jsonStr,
        };
      }
      return {
        content: normalizeContent(parsed.content),
        usage: parsed.usage || null,
        stopReason: parsed.stop_reason || "",
        model: parsed.model || "",
        _raw: jsonStr,
      };
    } catch (e) {
      // Not JSON — plain error string from err.Error()
      if (jsonStr) {
        return {
          content: [{ type: "text", text: jsonStr }],
          _isError: true,
          errorType: "proxy_error",
          _raw: jsonStr,
        };
      }
      return { _raw: jsonStr, _parseError: true };
    }
  }

  // HTML escape
  function esc(text) {
    var map = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#x27;" };
    return String(text).replace(/[&<>"']/g, function (c) { return map[c]; });
  }

  // Highlight search matches in escaped HTML text
  function highlightText(escapedHtml, query) {
    if (!query) return escapedHtml;
    var escapedQuery = esc(query).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    var re = new RegExp("(" + escapedQuery + ")", "gi");
    return escapedHtml.replace(re, '<mark class="mv-search-highlight">$1</mark>');
  }

  // Extract plain text from parts for search
  function partsToText(parts) {
    return parts.map(function (p) {
      if (p.type === "text") return p.text || "";
      if (p.type === "thinking") return p.thinking || "";
      if (p.type === "tool_use") return (p.name || "") + " " + JSON.stringify(p.input || {});
      if (p.type === "tool_result") {
        var c = p.content;
        if (typeof c === "string") return c;
        if (Array.isArray(c)) return c.map(function (x) { return x.text || ""; }).join(" ");
        return JSON.stringify(c || "");
      }
      return JSON.stringify(p);
    }).join(" ");
  }
// __CONTINUE_HERE__

  // Format JSON for raw view
  function formatJsonHighlight(str) {
    return VueUtils.highlightJson(VueUtils.formatJsonPretty(str));
  }

  window.VueComponents.MessageViewer = {
    name: "MessageViewer",
    props: {
      requestContent: { type: String, default: "" },
      responseContent: { type: String, default: "" },
      success: { type: Boolean, default: true },
    },
    setup: function (props) {
      var viewMode = ref("chat");
      var searchQuery = ref("");
      var roleFilter = ref("all");
      var typeFilter = ref("all");
      var collapsedThinking = ref({});
      var collapsedTools = ref({});
      var showTools = ref(false);

      var parsedRequest = computed(function () {
        return parseRequest(props.requestContent);
      });

      var parsedResponse = computed(function () {
        return parseResponse(props.responseContent);
      });

      // Auto-fallback to raw if parse failed
      var effectiveMode = computed(function () {
        if (viewMode.value === "raw") return "raw";
        var req = parsedRequest.value;
        var res = parsedResponse.value;
        if ((!req || req._parseError) && (!res || res._parseError)) return "raw";
        return "chat";
      });

      // All messages (request messages + response as final assistant message)
      var allMessages = computed(function () {
        var msgs = [];
        var req = parsedRequest.value;
        if (req && !req._parseError && req.messages) {
          msgs = msgs.concat(req.messages);
        }
        var res = parsedResponse.value;
        if (res && !res._parseError && res.content) {
          msgs.push({ role: "assistant", parts: res.content, _isResponse: true, _isError: !!res._isError, _errorType: res.errorType || "" });
        }
        return msgs;
      });

      // Filtered messages
      var filteredMessages = computed(function () {
        return allMessages.value.filter(function (msg) {
          // Role filter
          if (roleFilter.value !== "all" && msg.role !== roleFilter.value) return false;
          // Type filter
          if (typeFilter.value !== "all") {
            var hasType = msg.parts.some(function (p) { return p.type === typeFilter.value; });
            if (!hasType) return false;
          }
          // Search filter
          if (searchQuery.value) {
            var text = partsToText(msg.parts).toLowerCase();
            if (text.indexOf(searchQuery.value.toLowerCase()) === -1) return false;
          }
          return true;
        });
      });

      // Search match count
      var searchMatchCount = computed(function () {
        if (!searchQuery.value) return 0;
        var q = searchQuery.value.toLowerCase();
        var count = 0;
        allMessages.value.forEach(function (msg) {
          var text = partsToText(msg.parts).toLowerCase();
          var idx = 0;
          while ((idx = text.indexOf(q, idx)) !== -1) { count++; idx += q.length; }
        });
        // Also search system
        var req = parsedRequest.value;
        if (req && req.system) {
          var sysText = req.system.toLowerCase();
          var si = 0;
          while ((si = sysText.indexOf(q, si)) !== -1) { count++; si += q.length; }
        }
        return count;
      });

      // Content type counts for filter badges
      var typeCounts = computed(function () {
        var counts = { text: 0, tool_use: 0, tool_result: 0, thinking: 0, image: 0 };
        allMessages.value.forEach(function (msg) {
          msg.parts.forEach(function (p) {
            if (counts[p.type] !== undefined) counts[p.type]++;
          });
        });
        return counts;
      });

      function toggleThinking(key) {
        var obj = Object.assign({}, collapsedThinking.value);
        obj[key] = !obj[key];
        collapsedThinking.value = obj;
      }

      function toggleToolInput(key) {
        var obj = Object.assign({}, collapsedTools.value);
        obj[key] = !obj[key];
        collapsedTools.value = obj;
      }

      function renderPartText(text) {
        var escaped = esc(text || "");
        return highlightText(escaped, searchQuery.value);
      }

      function formatToolInput(input) {
        if (!input) return "{}";
        try {
          return JSON.stringify(input, null, 2);
        } catch (e) {
          return String(input);
        }
      }

      function renderToolResultContent(content) {
        if (typeof content === "string") return esc(content);
        if (Array.isArray(content)) {
          return content.map(function (c) { return esc(c.text || JSON.stringify(c)); }).join("\n");
        }
        return esc(JSON.stringify(content || ""));
      }

      return {
        viewMode: viewMode,
        searchQuery: searchQuery,
        roleFilter: roleFilter,
        typeFilter: typeFilter,
        collapsedThinking: collapsedThinking,
        collapsedTools: collapsedTools,
        showTools: showTools,
        parsedRequest: parsedRequest,
        parsedResponse: parsedResponse,
        effectiveMode: effectiveMode,
        allMessages: allMessages,
        filteredMessages: filteredMessages,
        searchMatchCount: searchMatchCount,
        typeCounts: typeCounts,
        toggleThinking: toggleThinking,
        toggleToolInput: toggleToolInput,
        renderPartText: renderPartText,
        formatToolInput: formatToolInput,
        renderToolResultContent: renderToolResultContent,
        formatJsonHighlight: formatJsonHighlight,
        esc: esc,
      };
    },
// __CONTINUE_TEMPLATE__
    template:
      '<div class="mv-container">\
        <!-- Toolbar -->\
        <div class="mv-toolbar">\
          <div class="mv-toolbar-row">\
            <div class="mv-mode-tabs">\
              <button class="mv-tab" :class="{ active: viewMode === \'chat\' }" @click="viewMode = \'chat\'">对话模式</button>\
              <button class="mv-tab" :class="{ active: viewMode === \'raw\' }" @click="viewMode = \'raw\'">原始 JSON</button>\
            </div>\
            <div class="mv-search" v-show="effectiveMode === \'chat\'">\
              <svg class="mv-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>\
              <input type="text" class="mv-search-input" v-model="searchQuery" placeholder="搜索消息...">\
              <span class="mv-search-count" v-show="searchQuery">{{ searchMatchCount }} 个匹配</span>\
            </div>\
          </div>\
          <div class="mv-toolbar-row mv-filters" v-show="effectiveMode === \'chat\'">\
            <div class="mv-filter-group">\
              <span class="mv-filter-label">角色:</span>\
              <button class="mv-pill" :class="{ active: roleFilter === \'all\' }" @click="roleFilter = \'all\'">全部</button>\
              <button class="mv-pill" :class="{ active: roleFilter === \'user\' }" @click="roleFilter = \'user\'">用户</button>\
              <button class="mv-pill" :class="{ active: roleFilter === \'assistant\' }" @click="roleFilter = \'assistant\'">助手</button>\
            </div>\
            <div class="mv-filter-group">\
              <span class="mv-filter-label">类型:</span>\
              <button class="mv-pill" :class="{ active: typeFilter === \'all\' }" @click="typeFilter = \'all\'">全部</button>\
              <button class="mv-pill" :class="{ active: typeFilter === \'text\' }" @click="typeFilter = \'text\'">文本<span class="mv-pill-count" v-show="typeCounts.text">({{ typeCounts.text }})</span></button>\
              <button class="mv-pill" :class="{ active: typeFilter === \'tool_use\' }" @click="typeFilter = \'tool_use\'" v-show="typeCounts.tool_use">工具<span class="mv-pill-count">({{ typeCounts.tool_use }})</span></button>\
              <button class="mv-pill" :class="{ active: typeFilter === \'thinking\' }" @click="typeFilter = \'thinking\'" v-show="typeCounts.thinking">思考<span class="mv-pill-count">({{ typeCounts.thinking }})</span></button>\
              <button class="mv-pill" :class="{ active: typeFilter === \'tool_result\' }" @click="typeFilter = \'tool_result\'" v-show="typeCounts.tool_result">工具结果<span class="mv-pill-count">({{ typeCounts.tool_result }})</span></button>\
            </div>\
          </div>\
        </div>\
        <!-- Chat mode -->\
        <div v-if="effectiveMode === \'chat\'" class="mv-chat-view">\
          <!-- Request params summary -->\
          <div class="mv-params" v-if="parsedRequest && !parsedRequest._parseError">\
            <span class="mv-param" v-if="parsedRequest.model"><b>model:</b> {{ parsedRequest.model }}</span>\
            <span class="mv-param" v-if="parsedRequest.maxTokens"><b>max_tokens:</b> {{ parsedRequest.maxTokens }}</span>\
            <span class="mv-param" v-if="parsedRequest.stream"><b>stream</b></span>\
            <span class="mv-param" v-if="parsedRequest.temperature != null"><b>temp:</b> {{ parsedRequest.temperature }}</span>\
            <span class="mv-param" v-if="parsedRequest.thinking"><b>thinking:</b> enabled</span>\
            <span class="mv-param mv-param-tools" v-if="parsedRequest.tools && parsedRequest.tools.length" @click="showTools = !showTools" style="cursor:pointer;"><b>tools:</b> {{ parsedRequest.tools.length }} 个 {{ showTools ? "▲" : "▼" }}</span>\
          </div>\
          <!-- Tools detail -->\
          <div class="mv-tools-detail" v-if="showTools && parsedRequest && parsedRequest.tools && parsedRequest.tools.length">\
            <div class="mv-tool-item" v-for="(tool, ti) in parsedRequest.tools" :key="ti">\
              <span class="mv-tool-name">{{ tool.name }}</span>\
              <span class="mv-tool-desc" v-if="tool.description">{{ tool.description }}</span>\
            </div>\
          </div>\
          <!-- System prompt -->\
          <div class="mv-system" v-if="parsedRequest && parsedRequest.system && (!searchQuery || parsedRequest.system.toLowerCase().indexOf(searchQuery.toLowerCase()) !== -1)">\
            <div class="mv-system-label">System Prompt</div>\
            <div class="mv-system-text" v-html="renderPartText(parsedRequest.system)"></div>\
          </div>\
          <!-- Messages -->\
          <div class="mv-messages" v-if="filteredMessages.length">\
            <div v-for="(msg, mi) in filteredMessages" :key="mi" class="mv-message" :class="\'mv-message--\' + msg.role">\
              <div class="mv-role-badge" :class="[\'mv-role--\' + msg.role, { \'mv-role--error\': msg._isError }]">\
                <span v-if="msg._isError">Error</span>\
                <span v-else-if="msg.role === \'user\'">User</span>\
                <span v-else-if="msg.role === \'assistant\'">{{ msg._isResponse ? "Response" : "Assistant" }}</span>\
                <span v-else>{{ msg.role }}</span>\
              </div>\
              <div class="mv-bubble" :class="{ \'mv-bubble--error\': msg._isError }">\
                <div class="mv-error-type" v-if="msg._isError && msg._errorType">{{ msg._errorType }}</div>\
                <div v-for="(part, pi) in msg.parts" :key="pi" class="mv-part" :class="\'mv-part--\' + part.type">\
                  <!-- text -->\
                  <div v-if="part.type === \'text\'" class="mv-text" v-html="renderPartText(part.text)"></div>\
                  <!-- thinking -->\
                  <div v-else-if="part.type === \'thinking\'" class="mv-thinking">\
                    <div class="mv-thinking-header" @click="toggleThinking(mi + \'-\' + pi)">\
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>\
                      <span>Thinking</span>\
                      <svg class="mv-chevron" :class="{ open: !collapsedThinking[mi + \'-\' + pi] }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                    </div>\
                    <div class="mv-thinking-body" v-show="!collapsedThinking[mi + \'-\' + pi]" v-html="renderPartText(part.thinking)"></div>\
                  </div>\
                  <!-- tool_use -->\
                  <div v-else-if="part.type === \'tool_use\'" class="mv-tool-use">\
                    <div class="mv-tool-use-header" @click="toggleToolInput(mi + \'-\' + pi)">\
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>\
                      <span>{{ part.name || "tool" }}</span>\
                      <span class="mv-tool-id" v-if="part.id">{{ part.id }}</span>\
                      <svg class="mv-chevron" :class="{ open: !collapsedTools[mi + \'-\' + pi] }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>\
                    </div>\
                    <pre class="mv-tool-input" v-show="!collapsedTools[mi + \'-\' + pi]">{{ formatToolInput(part.input) }}</pre>\
                  </div>\
                  <!-- tool_result -->\
                  <div v-else-if="part.type === \'tool_result\'" class="mv-tool-result">\
                    <div class="mv-tool-result-header">\
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>\
                      <span>Tool Result</span>\
                      <span class="mv-tool-id" v-if="part.tool_use_id">{{ part.tool_use_id }}</span>\
                      <span class="mv-tool-error" v-if="part.is_error">error</span>\
                    </div>\
                    <pre class="mv-tool-result-body" v-html="renderToolResultContent(part.content)"></pre>\
                  </div>\
                  <!-- image -->\
                  <div v-else-if="part.type === \'image\'" class="mv-image-placeholder">\
                    [图片: {{ (part.source && part.source.media_type) || "image" }}]\
                  </div>\
                  <!-- unknown type -->\
                  <div v-else class="mv-unknown">\
                    <span class="mv-type-badge">{{ part.type }}</span>\
                    <pre class="mv-unknown-json">{{ JSON.stringify(part, null, 2) }}</pre>\
                  </div>\
                </div>\
              </div>\
            </div>\
          </div>\
          <div v-else class="mv-empty">{{ searchQuery ? "无匹配结果" : "(无消息)" }}</div>\
          <!-- Response metadata -->\
          <div class="mv-response-meta" v-if="parsedResponse && !parsedResponse._parseError && parsedResponse.stopReason">\
            <span class="mv-meta-item"><b>stop_reason:</b> {{ parsedResponse.stopReason }}</span>\
            <span class="mv-meta-item" v-if="parsedResponse.usage"><b>input:</b> {{ (parsedResponse.usage.input_tokens || 0).toLocaleString() }}</span>\
            <span class="mv-meta-item" v-if="parsedResponse.usage"><b>output:</b> {{ (parsedResponse.usage.output_tokens || 0).toLocaleString() }}</span>\
          </div>\
        </div>\
        <!-- Raw JSON mode -->\
        <div v-else class="mv-raw-view">\
          <div class="mv-raw-section" v-if="requestContent">\
            <h4>请求内容</h4>\
            <pre class="mv-raw-json json-viewer" v-html="formatJsonHighlight(requestContent)"></pre>\
          </div>\
          <div class="mv-raw-section" v-if="responseContent">\
            <h4>{{ success ? \'响应内容\' : \'错误原因\' }}</h4>\
            <pre class="mv-raw-json json-viewer" v-html="formatJsonHighlight(responseContent)"></pre>\
          </div>\
          <div v-if="!requestContent && !responseContent" class="mv-empty">(无内容)</div>\
        </div>\
      </div>',
  };
})();
