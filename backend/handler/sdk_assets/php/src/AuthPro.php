<?php
/**
 * AuthPro 客户端 SDK（PHP）
 *
 * 公共 API：boot / verify / checkUpdate / ads / pluginSourceUrl
 * 应用差异只写在 config.json，不要改本库源码。
 */
if (class_exists('AuthPro', false)) {
    return;
}

class AuthPro
{
    /** @var array */
    private static $config = array();

    /** @var bool */
    private static $booted = false;

    /**
     * 加载配置并按模块执行启动逻辑。
     * license / piracy 开启时，校验失败会中断（盗版模块展示拦截页）。
     *
     * @param array|string $config 配置数组，或指向 config.json 的路径
     * @return bool
     */
    public static function boot($config = array())
    {
        self::loadConfig($config);
        self::$booted = true;
        if (self::moduleEnabled('license') || self::moduleEnabled('piracy')) {
            $result = self::verify();
            if (empty($result['ok'])) {
                self::deny($result);
            }
        }
        return true;
    }

    /**
     * 仅做授权校验，不强制退出进程。
     * 返回结构：{ ok, code, message, data }
     *
     * @param array $overrides 可覆盖 licenseKey / domain / serverIp
     * @return array
     */
    public static function verify($overrides = array())
    {
        self::ensureConfig();
        $ctx = self::requestContext($overrides);
        $timestamp = time();
        $payload = array(
            'appKey' => self::cfg('appKey', ''),
            'domain' => $ctx['domain'],
            'serverIp' => $ctx['serverIp'],
            'licenseKey' => $ctx['licenseKey'],
            'timestamp' => $timestamp,
            'signVersion' => 'v2',
            'sign' => self::v2Sign(array(
                'v2',
                self::cfg('appKey', ''),
                $ctx['licenseKey'],
                $ctx['domain'],
                $ctx['serverIp'],
                (string)$timestamp,
            )),
        );
        $response = self::httpJson('POST', '/api/license/verify', $payload);
        return self::normalizeResult($response);
    }

    /**
     * 在线更新检查。
     *
     * @param string|null $currentVersion
     * @param array $overrides
     * @return array
     */
    public static function checkUpdate($currentVersion = null, $overrides = array())
    {
        self::ensureConfig();
        $ctx = self::requestContext($overrides);
        if ($currentVersion === null || $currentVersion === '') {
            $currentVersion = self::cfg('appVersion', '1.0.0');
        }
        $timestamp = time();
        $payload = array(
            'appKey' => self::cfg('appKey', ''),
            'currentVersion' => (string)$currentVersion,
            'domain' => $ctx['domain'],
            'serverIp' => $ctx['serverIp'],
            'licenseKey' => $ctx['licenseKey'],
            'timestamp' => $timestamp,
            'signVersion' => 'v2',
            'sign' => self::v2Sign(array(
                'v2',
                self::cfg('appKey', ''),
                (string)$currentVersion,
                $ctx['licenseKey'],
                $ctx['domain'],
                $ctx['serverIp'],
                (string)$timestamp,
            )),
        );
        $response = self::httpJson('POST', '/api/app/version/check', $payload);
        if (isset($response['data']['downloadUrl']) && is_string($response['data']['downloadUrl'])
            && strpos($response['data']['downloadUrl'], 'http') !== 0) {
            $response['data']['downloadUrl'] = rtrim(self::cfg('baseUrl', ''), '/') . $response['data']['downloadUrl'];
        }
        return self::normalizeResult($response, false);
    }

    /**
     * 拉取广告位：home-banner / sidebar / popup
     *
     * @param string $slot
     * @return array
     */
    public static function ads($slot = 'home-banner')
    {
        self::ensureConfig();
        $slot = trim((string)$slot);
        if ($slot === '') {
            $slot = 'home-banner';
        }
        $response = self::httpJson('GET', '/api/v1/public/advertisements?position=' . rawurlencode($slot), null);
        if (isset($response['data']) && is_array($response['data'])) {
            return $response['data'];
        }
        return array('records' => array(), 'placeholder' => null);
    }

    /**
     * 本应用隔离的插件源清单 URL。
     *
     * @return string
     */
    public static function pluginSourceUrl()
    {
        self::ensureConfig();
        $base = rtrim(self::cfg('baseUrl', ''), '/');
        $appKey = trim((string)self::cfg('appKey', ''));
        return $base . '/software-source/' . rawurlencode($appKey) . '/index.json';
    }

    /**
     * @param array|string $config
     */
    public static function loadConfig($config)
    {
        if (is_string($config)) {
            $path = $config;
            if (!is_file($path)) {
                throw new InvalidArgumentException('找不到配置文件：' . $path);
            }
            $raw = file_get_contents($path);
            $decoded = json_decode($raw ? $raw : '{}', true);
            if (!is_array($decoded)) {
                throw new InvalidArgumentException('config.json 不是合法 JSON');
            }
            self::$config = $decoded;
            return;
        }
        if (!is_array($config)) {
            $config = array();
        }
        self::$config = array_merge(self::$config, $config);
    }

    private static function ensureConfig()
    {
        if (!self::$booted && empty(self::$config)) {
            throw new RuntimeException('请先调用 AuthPro::boot($config) 或 AuthPro::loadConfig($config)');
        }
    }

    private static function cfg($key, $default = null)
    {
        return array_key_exists($key, self::$config) ? self::$config[$key] : $default;
    }

    private static function moduleEnabled($name)
    {
        $modules = self::cfg('modules', array());
        if (!is_array($modules)) {
            return false;
        }
        if (array_key_exists($name, $modules)) {
            return (bool)$modules[$name];
        }
        // 兼容 modules: ["license","ads"] 数组写法
        foreach ($modules as $item) {
            if (is_string($item) && strtolower($item) === strtolower($name)) {
                return true;
            }
        }
        return false;
    }

    private static function requestContext($overrides = array())
    {
        if (!is_array($overrides)) {
            $overrides = array();
        }
        $licenseKey = '';
        $domain = '';
        $serverIp = '';
        if (isset($overrides['licenseKey'])) {
            $licenseKey = trim((string)$overrides['licenseKey']);
        } elseif (self::cfg('licenseKey') !== null) {
            $licenseKey = trim((string)self::cfg('licenseKey', ''));
        }
        if (isset($overrides['domain'])) {
            $domain = self::normalizeDomain($overrides['domain']);
        } elseif (self::cfg('domain')) {
            $domain = self::normalizeDomain(self::cfg('domain'));
        }
        if (isset($overrides['serverIp'])) {
            $serverIp = self::normalizeServerIP($overrides['serverIp']);
        } elseif (self::cfg('serverIp')) {
            $serverIp = self::normalizeServerIP(self::cfg('serverIp'));
        }
        if ($domain === '' && !empty($_SERVER['HTTP_HOST'])) {
            $domain = self::normalizeDomain($_SERVER['HTTP_HOST']);
        }
        if ($serverIp === '' && !empty($_SERVER['SERVER_ADDR'])) {
            $serverIp = self::normalizeServerIP($_SERVER['SERVER_ADDR']);
        }
        return array(
            'licenseKey' => $licenseKey,
            'domain' => $domain,
            'serverIp' => $serverIp,
        );
    }

    private static function normalizeDomain($value)
    {
        $value = strtolower(trim((string)$value));
        $value = preg_replace('#^https?://#', '', $value);
        $value = preg_replace('#/.*$#', '', $value);
        if ($value !== '' && $value[0] === '[') {
            $value = trim($value, '[]');
        }
        $value = preg_replace('#:\d+$#', '', $value);
        return rtrim($value, '.');
    }

    private static function normalizeServerIP($value)
    {
        $value = trim((string)$value);
        $zone = strrpos($value, '%');
        if ($zone !== false) {
            $value = substr($value, 0, $zone);
        }
        return trim($value, '[]');
    }

    private static function v2Sign($parts)
    {
        $secret = (string)self::cfg('appSecret', '');
        if ($secret === '') {
            throw new RuntimeException('缺少 appSecret，无法签名。请在服务端 config.json 中配置。');
        }
        return hash_hmac('sha256', implode("\n", $parts), $secret);
    }

    private static function normalizeResult($response, $requirePass = true)
    {
        if (!is_array($response)) {
            $response = array();
        }
        $code = isset($response['code']) ? (int)$response['code'] : 0;
        $message = '';
        if (!empty($response['msg'])) {
            $message = (string)$response['msg'];
        } elseif (!empty($response['message'])) {
            $message = (string)$response['message'];
        }
        $data = isset($response['data']) ? $response['data'] : null;
        $ok = $code === 200;
        if ($requirePass) {
            $ok = $ok && is_array($data) && isset($data['result']) && $data['result'] === 'pass';
        }
        return array(
            'ok' => $ok,
            'code' => $code,
            'message' => $message,
            'data' => $data,
        );
    }

    private static function deny($result)
    {
        $reason = '';
        if (is_array($result)) {
            if (!empty($result['message'])) {
                $reason = (string)$result['message'];
            } elseif (is_array($result['data']) && !empty($result['data']['reason'])) {
                $reason = (string)$result['data']['reason'];
            }
        }
        $title = '授权无效';
        $hint = '请检查授权码、域名是否与后台登记一致。';
        if (self::moduleEnabled('piracy')) {
            $title = '未授权访问';
            $hint = '当前站点未通过授权校验。授权站会记录未授权命中，请联系软件提供方开通正版。';
        }
        if (PHP_SAPI === 'cli') {
            fwrite(STDERR, $title . ': ' . $reason . PHP_EOL);
            exit(1);
        }
        if (!headers_sent()) {
            http_response_code(403);
            header('Content-Type: text/html; charset=utf-8');
        }
        echo '<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>'
            . htmlspecialchars($title, ENT_QUOTES, 'UTF-8')
            . '</title><style>body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}main{max-width:520px;padding:32px;background:#1e293b;border-radius:16px}h1{margin:0 0 12px;font-size:24px}p{line-height:1.7;color:#cbd5e1}</style></head><body><main><h1>'
            . htmlspecialchars($title, ENT_QUOTES, 'UTF-8')
            . '</h1><p>'
            . htmlspecialchars($hint, ENT_QUOTES, 'UTF-8')
            . '</p><p>'
            . htmlspecialchars($reason, ENT_QUOTES, 'UTF-8')
            . '</p></main></body></html>';
        exit;
    }

    private static function httpJson($method, $path, $payload)
    {
        $base = rtrim(self::cfg('baseUrl', ''), '/');
        $url = $base . $path;
        $body = $payload === null ? null : json_encode($payload);
        if (function_exists('curl_init')) {
            $ch = curl_init($url);
            $headers = array('Accept: application/json');
            $opts = array(
                CURLOPT_RETURNTRANSFER => true,
                CURLOPT_TIMEOUT => 8,
                CURLOPT_FOLLOWLOCATION => true,
            );
            if ($method === 'POST') {
                $headers[] = 'Content-Type: application/json';
                $opts[CURLOPT_POST] = true;
                $opts[CURLOPT_POSTFIELDS] = $body;
            }
            $opts[CURLOPT_HTTPHEADER] = $headers;
            curl_setopt_array($ch, $opts);
            $raw = curl_exec($ch);
            curl_close($ch);
        } else {
            $header = "Accept: application/json\r\n";
            $http = array('method' => $method, 'timeout' => 8, 'ignore_errors' => true);
            if ($method === 'POST') {
                $header .= "Content-Type: application/json\r\n";
                $http['content'] = $body;
            }
            $http['header'] = $header;
            $raw = @file_get_contents($url, false, stream_context_create(array('http' => $http)));
        }
        $decoded = json_decode($raw ? $raw : '{}', true);
        return is_array($decoded) ? $decoded : array();
    }
}
