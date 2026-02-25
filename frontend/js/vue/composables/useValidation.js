/**
 * useValidation 组合式函数
 * 提供表单验证能力，支持多种内置验证规则和自定义规则
 */
;(function () {
    'use strict';

    window.VueComposables = window.VueComposables || {};

    var reactive = Vue.reactive;
    var computed = Vue.computed;

    // ========== 内置验证器 ==========

    var validators = {
        /** 必填 */
        required: function (value, message) {
            if (message === undefined) message = '此字段为必填项';
            if (value === null || value === undefined || value === '') return message;
            if (Array.isArray(value) && value.length === 0) return message;
            return null;
        },

        /** 最小长度 */
        minLength: function (value, min, message) {
            if (!value) return null;
            if (value.length < min) {
                return message || '最少需要 ' + min + ' 个字符';
            }
            return null;
        },

        /** 最大长度 */
        maxLength: function (value, max, message) {
            if (!value) return null;
            if (value.length > max) {
                return message || '最多允许 ' + max + ' 个字符';
            }
            return null;
        },

        /** 邮箱格式 */
        email: function (value, message) {
            if (!value) return null;
            if (message === undefined) message = '请输入有效的邮箱地址';
            var re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
            return re.test(value) ? null : message;
        },

        /** URL 格式 */
        url: function (value, message) {
            if (!value) return null;
            if (message === undefined) message = '请输入有效的 URL';
            try {
                new URL(value);
                return null;
            } catch (e) {
                return message;
            }
        },

        /** 数字范围 */
        range: function (value, min, max, message) {
            if (value === null || value === undefined || value === '') return null;
            var num = Number(value);
            if (isNaN(num)) return '请输入有效的数字';
            if (min !== undefined && num < min) {
                return message || '数值不能小于 ' + min;
            }
            if (max !== undefined && num > max) {
                return message || '数值不能大于 ' + max;
            }
            return null;
        },

        /** 正整数 */
        positiveInt: function (value, message) {
            if (value === null || value === undefined || value === '') return null;
            if (message === undefined) message = '请输入正整数';
            var num = Number(value);
            return (Number.isInteger(num) && num > 0) ? null : message;
        },

        /** 正则匹配 */
        pattern: function (value, regex, message) {
            if (!value) return null;
            if (message === undefined) message = '格式不正确';
            return regex.test(value) ? null : message;
        },

        /** 用户名格式 */
        username: function (value, message) {
            if (!value) return null;
            if (message === undefined) message = '用户名只能包含字母、数字和下划线';
            return /^[a-zA-Z0-9_]+$/.test(value) ? null : message;
        },

        /** 密码强度 */
        password: function (value, options) {
            if (!value) return null;
            if (options === undefined) options = {};
            var minLen = options.minLength || 6;
            var requireNumber = options.requireNumber || false;
            var requireSpecial = options.requireSpecial || false;

            if (value.length < minLen) {
                return '密码至少需要 ' + minLen + ' 个字符';
            }
            if (requireNumber && !/\d/.test(value)) {
                return '密码需要包含至少一个数字';
            }
            if (requireSpecial && !/[!@#$%^&*(),.?":{}|<>]/.test(value)) {
                return '密码需要包含至少一个特殊字符';
            }
            return null;
        },

        /** 确认密码 */
        confirmPassword: function (value, password, message) {
            if (!value) return null;
            if (message === undefined) message = '两次输入的密码不一致';
            return value !== password ? message : null;
        }
    };

    // ========== 表单验证器工厂 ==========

    /**
     * 创建表单验证器
     * @param {Object} rules - 验证规则，格式: { fieldName: [rule1, rule2, ...] }
     *   rule 可以是:
     *     - 字符串: 'required' → 调用 validators.required
     *     - 对象: { type: 'minLength', value: 3, message: '...' }
     *     - 函数: (value, allValues) => errorMsg | null
     * @returns {Object} 响应式验证器对象
     */
    function createFormValidator(rules) {
        var state = reactive({
            errors: {},
            touched: {}
        });

        /**
         * 根据规则定义调用对应验证器
         */
        function applyRule(rule, value, allValues) {
            // 函数类型规则
            if (typeof rule === 'function') {
                return rule(value, allValues);
            }
            // 字符串类型规则（简写）
            if (typeof rule === 'string') {
                var v = validators[rule];
                return v ? v(value) : null;
            }
            // 对象类型规则
            if (typeof rule === 'object' && rule !== null) {
                var type = rule.type;
                var validator = validators[type];
                if (!validator) return null;

                if (type === 'confirmPassword') {
                    return validator(value, allValues[rule.field], rule.message);
                } else if (type === 'range') {
                    return validator(value, rule.min, rule.max, rule.message);
                } else if (type === 'minLength' || type === 'maxLength') {
                    return validator(value, rule.value, rule.message);
                } else if (type === 'pattern') {
                    return validator(value, rule.regex, rule.message);
                } else if (type === 'password') {
                    return validator(value, rule);
                } else {
                    return validator(value, rule.message);
                }
            }
            return null;
        }

        return {
            /** 响应式错误对象 */
            errors: state.errors,
            /** 响应式已触摸字段 */
            touched: state.touched,

            /**
             * 验证单个字段
             * @param {string} field - 字段名
             * @param {*} value - 字段值
             * @param {Object} allValues - 所有字段值（用于跨字段验证）
             * @returns {string|null} 错误消息或 null
             */
            validateField: function (field, value, allValues) {
                if (allValues === undefined) allValues = {};
                var fieldRules = rules[field];
                if (!fieldRules) return null;

                for (var i = 0; i < fieldRules.length; i++) {
                    var error = applyRule(fieldRules[i], value, allValues);
                    if (error) {
                        state.errors[field] = error;
                        return error;
                    }
                }

                delete state.errors[field];
                return null;
            },

            /**
             * 验证所有字段
             * @param {Object} values - 所有字段值
             * @returns {boolean} 是否全部通过
             */
            validateAll: function (values) {
                state.errors = {};
                var isValid = true;
                var fields = Object.keys(rules);

                for (var i = 0; i < fields.length; i++) {
                    var field = fields[i];
                    state.touched[field] = true;
                    var error = this.validateField(field, values[field], values);
                    if (error) isValid = false;
                }

                return isValid;
            },

            /**
             * 获取字段错误（仅已触摸的字段）
             * @param {string} field - 字段名
             * @returns {string|null}
             */
            getError: function (field) {
                return state.touched[field] ? (state.errors[field] || null) : null;
            },

            /**
             * 检查字段是否有错误（仅已触摸的字段）
             * @param {string} field - 字段名
             * @returns {boolean}
             */
            hasError: function (field) {
                return !!(state.touched[field] && state.errors[field]);
            },

            /**
             * 检查表单是否有效
             * @returns {boolean}
             */
            isValid: function () {
                return Object.keys(state.errors).length === 0;
            },

            /**
             * 重置验证状态
             */
            reset: function () {
                var errorKeys = Object.keys(state.errors);
                for (var i = 0; i < errorKeys.length; i++) {
                    delete state.errors[errorKeys[i]];
                }
                var touchedKeys = Object.keys(state.touched);
                for (var j = 0; j < touchedKeys.length; j++) {
                    delete state.touched[touchedKeys[j]];
                }
            },

            /**
             * 标记字段为已触摸
             * @param {string} field - 字段名
             */
            touch: function (field) {
                state.touched[field] = true;
            }
        };
    }

    // 导出
    window.VueComposables.useValidation = {
        validators: validators,
        createFormValidator: createFormValidator
    };
})();
