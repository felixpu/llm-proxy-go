/**
 * HelpPage - 使用帮助页面
 * 从 Alpine.js help.html 移植到 Vue 3
 */
window.VuePages = window.VuePages || {};

(function () {
  "use strict";

  var computed = Vue.computed;
  var inject = Vue.inject;

  var baseUrl = window.location.origin;

  window.VuePages.HelpPage = {
    name: "HelpPage",
    setup: function () {
      var toastStore = inject("toastStore");

      var envConfig = computed(function () {
        return (
          'export ANTHROPIC_BASE_URL="' +
          baseUrl +
          '"\nexport ANTHROPIC_AUTH_TOKEN="your-api-key"'
        );
      });

      var shellConfig = computed(function () {
        return (
          '# LLM Proxy 配置\nexport ANTHROPIC_BASE_URL="' +
          baseUrl +
          '"\nexport ANTHROPIC_AUTH_TOKEN="your-api-key"'
        );
      });

      var claudeConfig = computed(function () {
        return (
          'claude config set --global apiBaseUrl "' +
          baseUrl +
          '"\nclaude config set --global apiKey "your-api-key"'
        );
      });

      var claudeSettingConfig = computed(function () {
        return (
          '{\n    "ANTHROPIC_BASE_URL": "' +
          baseUrl +
          '",\n    "ANTHROPIC_AUTH_TOKEN": "your-api-key"\n}'
        );
      });

      function copyText(text) {
        VueUtils.copyToClipboard(text).then(function () {
          toastStore.success("已复制到剪贴板");
        });
      }

      return {
        envConfig: envConfig,
        shellConfig: shellConfig,
        claudeConfig: claudeConfig,
        claudeSettingConfig: claudeSettingConfig,
        copyText: copyText,
      };
    },
    template:
      '\
<div class="help-page">\
    <!-- 快速开始 -->\
    <div class="section">\
        <h3>快速开始</h3>\
        <div class="help-content">\
            <p>LLM Proxy 是一个智能 AI 模型代理服务，支持多端点负载均衡和基于内容的智能路由。兼容 Anthropic Messages API。</p>\
            <div class="steps">\
                <div class="step">\
                    <span class="step-number">1</span>\
                    <div class="step-content">\
                        <strong>生成 API Key</strong>\
                        <p>在 <a href="#/api-keys">API Keys</a> 页面创建一个新的 API Key</p>\
                    </div>\
                </div>\
                <div class="step">\
                    <span class="step-number">2</span>\
                    <div class="step-content">\
                        <strong>配置客户端</strong>\
                        <p>将代理地址和 API Key 配置到你的客户端（如 Claude Code）</p>\
                    </div>\
                </div>\
                <div class="step">\
                    <span class="step-number">3</span>\
                    <div class="step-content">\
                        <strong>开始使用</strong>\
                        <p>代理会自动根据请求内容选择最合适的模型</p>\
                    </div>\
                </div>\
            </div>\
        </div>\
    </div>\
    <!-- API Key 管理 -->\
    <div class="section">\
        <h3>API Key 管理</h3>\
        <div class="help-content">\
            <p>API Key 用于验证你对代理服务的访问权限。</p>\
            <ul class="help-list">\
                <li><strong>创建 Key：</strong>在 <a href="#/api-keys">API Keys</a> 页面点击"创建 API Key"，输入名称后即可生成</li>\
                <li><strong>保存 Key：</strong>API Key 只在创建时显示一次，请立即复制保存</li>\
                <li><strong>删除 Key：</strong>如果 Key 泄露，请立即删除并创建新的</li>\
            </ul>\
        </div>\
    </div>\
    <!-- Claude Code 配置 -->\
    <div class="section">\
        <h3>在 Claude Code 中使用</h3>\
        <div class="help-content">\
            <p>Claude Code 是 Anthropic 官方的 CLI 工具。配置代理后可以使用本服务的智能路由功能。</p>\
            <h4>方式一：环境变量配置</h4>\
            <p>在终端中设置环境变量：</p>\
            <div class="code-block">\
                <pre>{{ envConfig }}</pre>\
                <button class="btn btn-xs copy-btn" @click="copyText(envConfig)">复制</button>\
            </div>\
            <p>或添加到 shell 配置文件（<code>~/.bashrc</code> 或 <code>~/.zshrc</code>）：</p>\
            <div class="code-block">\
                <pre>{{ shellConfig }}</pre>\
                <button class="btn btn-xs copy-btn" @click="copyText(shellConfig)">复制</button>\
            </div>\
            <h4>方式二：Claude Code 设置</h4>\
            <p>运行以下命令配置：</p>\
            <div class="code-block">\
                <pre>{{ claudeConfig }}</pre>\
                <button class="btn btn-xs copy-btn" @click="copyText(claudeConfig)">复制</button>\
            </div>\
            <p>~/.claude/setting.json配置：</p>\
            <div class="code-block">\
                <pre>{{ claudeSettingConfig }}</pre>\
                <button class="btn btn-xs copy-btn" @click="copyText(claudeSettingConfig)">复制</button>\
            </div>\
            <div class="help-note">\
                <strong>注意：</strong>将 <code>your-api-key</code> 替换为你在 <a href="#/api-keys">API Keys</a> 页面生成的实际 Key。\
            </div>\
        </div>\
    </div>\
    <!-- 智能路由 -->\
    <div class="section">\
        <h3>智能路由</h3>\
        <div class="help-content">\
            <p>代理会根据请求内容自动选择最合适的模型：</p>\
            <div class="table-container">\
                <table class="table">\
                    <thead>\
                        <tr>\
                            <th>任务类型</th>\
                            <th>角色</th>\
                            <th>适用场景</th>\
                        </tr>\
                    </thead>\
                    <tbody>\
                        <tr>\
                            <td><span class="role-simple">simple</span></td>\
                            <td>轻量模型</td>\
                            <td>简单查询、文件读取、格式转换</td>\
                        </tr>\
                        <tr>\
                            <td><span class="role-default">default</span></td>\
                            <td>平衡模型</td>\
                            <td>一般编程任务、代码修改</td>\
                        </tr>\
                        <tr>\
                            <td><span class="role-complex">complex</span></td>\
                            <td>高能模型</td>\
                            <td>架构设计、复杂重构、深度分析</td>\
                        </tr>\
                    </tbody>\
                </table>\
            </div>\
            <p>管理员可以在 <a href="#/routing">路由规则</a> 页面配置匹配模式。</p>\
        </div>\
    </div>\
    <!-- API 端点 -->\
    <div class="section">\
        <h3>API 端点</h3>\
        <div class="help-content">\
            <div class="table-container">\
                <table class="table">\
                    <thead>\
                        <tr>\
                            <th>端点</th>\
                            <th>方法</th>\
                            <th>说明</th>\
                        </tr>\
                    </thead>\
                    <tbody>\
                        <tr>\
                            <td><code>/v1/messages</code></td>\
                            <td>POST</td>\
                            <td>Anthropic Messages API 兼容端点</td>\
                        </tr>\
                        <tr>\
                            <td><code>/api/health</code></td>\
                            <td>GET</td>\
                            <td>端点健康状态</td>\
                        </tr>\
                        <tr>\
                            <td><code>/api/status</code></td>\
                            <td>GET</td>\
                            <td>系统运行状态</td>\
                        </tr>\
                        <tr>\
                            <td><code>/docs</code></td>\
                            <td>GET</td>\
                            <td>完整 API 文档（Swagger UI）</td>\
                        </tr>\
                    </tbody>\
                </table>\
            </div>\
        </div>\
    </div>\
</div>\
',
  };
})();
