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

    /** 授权响应签名的用途标记，签名原文第一行。 */
    const LICENSE_PROOF_KIND = 'auth-pro-license-v3';
    /** 校验通过的结果默认缓存 5 分钟。 */
    const DEFAULT_CACHE_TTL = 300;
    /** 连不上授权站时，最近一次验签通过的结果最多再用 72 小时（从授权站签名时间算起）。 */
    const DEFAULT_OFFLINE_GRACE = 259200;

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
     * 响应必须带授权站的 Ed25519 签名（config.json 的 publicKey）才算数；校验通过的结果缓存 cacheTtl 秒，
     * 连不上授权站时在 offlineGrace 秒内沿用上次通过的结果。
     *
     * @param array $overrides 可覆盖 licenseKey / domain / serverIp；浏览器代理转发时再传浏览器给的 nonce，
     *                         这时不读写本机缓存，结果原样交给浏览器验签
     * @return array
     */
    public static function verify($overrides = array())
    {
        self::ensureConfig();
        $publicKey = self::publicKey();
        if (!is_array($overrides)) {
            $overrides = array();
        }
        $nonce = isset($overrides['nonce']) ? trim((string)$overrides['nonce']) : '';
        $relay = $nonce !== '';
        if (!$relay) {
            $nonce = bin2hex(random_bytes(16));
        }
        $ctx = self::requestContext($overrides);
        $cacheFile = self::cachePath($ctx);
        $now = time();
        if (!$relay) {
            $entry = self::readCache($cacheFile, $ctx, $publicKey);
            if ($entry !== null && $now - $entry['savedAt'] < self::positiveOr(self::cfg('cacheTtl', 0), self::DEFAULT_CACHE_TTL)) {
                return self::entryResult($entry, 'cached');
            }
        }
        $payload = array(
            'appKey' => self::cfg('appKey', ''),
            'domain' => $ctx['domain'],
            'serverIp' => $ctx['serverIp'],
            'licenseKey' => $ctx['licenseKey'],
            'timestamp' => $now,
            'signVersion' => 'v3',
            'nonce' => $nonce,
            'sign' => self::v2Sign(array(
                'v3',
                self::cfg('appKey', ''),
                $ctx['licenseKey'],
                $ctx['domain'],
                $ctx['serverIp'],
                (string)$now,
                $nonce,
            )),
        );
        $response = self::httpJson('POST', '/api/license/verify', $payload);
        if ($response === null || self::verifyLicenseProof($publicKey, $ctx['licenseKey'], $nonce, $response) === null) {
            return self::offlineResult($cacheFile, $ctx, $publicKey, $relay, $response);
        }
        $result = self::normalizeResult($response);
        if (!$relay) {
            if ($result['ok']) {
                self::writeCache($cacheFile, array('savedAt' => $now, 'nonce' => $nonce, 'body' => $response));
            } elseif (is_file($cacheFile)) {
                // 授权站明确拒绝（签名有效）：立刻失效，不再用旧缓存放行。
                @unlink($cacheFile);
            }
        }
        return $result;
    }

    /**
     * 连不上授权站或响应验签不过：宽限期内沿用上次验签通过的结果，否则拒绝（data.unverified = true）。
     */
    private static function offlineResult($cacheFile, $ctx, $publicKey, $relay, $response)
    {
        if (!$relay) {
            $entry = self::readCache($cacheFile, $ctx, $publicKey);
            $now = time();
            $grace = self::positiveOr(self::cfg('offlineGrace', 0), self::DEFAULT_OFFLINE_GRACE);
            if ($entry !== null && $now <= $entry['serverTime'] + $grace && ($entry['expireTs'] === 0 || $now < $entry['expireTs'])) {
                return self::entryResult($entry, 'offline');
            }
        }
        $result = self::normalizeResult($response);
        $result['ok'] = false;
        $data = is_array($result['data']) ? $result['data'] : array();
        $data['unverified'] = true;
        $result['data'] = $data;
        if ($result['message'] === '') {
            $result['message'] = $response === null ? '无法连接授权站' : '授权响应无法验证';
        }
        return $result;
    }

    private static function publicKey()
    {
        if (!function_exists('sodium_crypto_sign_verify_detached')) {
            throw new RuntimeException('授权校验需要 PHP sodium 扩展（PHP 7.2 起自带）。主机没有时可 composer require paragonie/sodium_compat。');
        }
        $raw = base64_decode(trim((string)self::cfg('publicKey', '')), true);
        if ($raw === false || strlen($raw) !== 32) {
            throw new RuntimeException('缺少或无效的 publicKey，请在授权站后台重新下载接入包，或从「接入开发」页复制授权响应公钥填入 config.json');
        }
        return $raw;
    }

    /**
     * 核对 data.proof：appKey、nonce、授权码哈希必须是自己这次发出的，再按接入文档的规则拼原文验签。
     * 域名和 IP 取 proof 里授权站规范化后的值，它们已在 v3 请求签名里和 nonce 绑在一起。
     * 通过返回签名里的服务器时间，不通过返回 null。
     */
    private static function verifyLicenseProof($publicKey, $licenseKey, $nonce, $body)
    {
        if (!is_array($body) || !isset($body['data']['proof']) || !is_array($body['data']['proof'])) {
            return null;
        }
        $data = $body['data'];
        $proof = $data['proof'];
        $appKey = (string)self::cfg('appKey', '');
        $keyHash = $licenseKey === '' ? '' : hash('sha256', $licenseKey);
        if (self::text($proof, 'appKey') !== $appKey || self::text($proof, 'nonce') !== $nonce
            || self::text($proof, 'licenseKeyHash') !== $keyHash) {
            return null;
        }
        if (!isset($proof['serverTime']) || !is_int($proof['serverTime'])) {
            return null;
        }
        $signature = self::text($proof, 'signature');
        if (strpos($signature, 'ed25519:') !== 0) {
            return null;
        }
        $sig = base64_decode(substr($signature, 8), true);
        if ($sig === false || strlen($sig) !== 64) {
            return null;
        }
        $fields = array(
            'appKey' => $appKey,
            'domain' => self::text($proof, 'domain'),
            'serverIp' => self::text($proof, 'serverIp'),
            'licenseKeyHash' => $keyHash,
            'nonce' => $nonce,
            'serverTime' => (string)$proof['serverTime'],
            'result' => self::text($data, 'result'),
            'reason' => self::text($data, 'reason'),
            'expireTs' => isset($data['expireTs']) && is_int($data['expireTs']) ? (string)$data['expireTs'] : '',
        );
        $message = self::LICENSE_PROOF_KIND . "\n";
        foreach ($fields as $key => $value) {
            $message .= $key . '=' . str_replace(array("\r", "\n"), ' ', $value) . "\n";
        }
        if (!sodium_crypto_sign_verify_detached($sig, $message, $publicKey)) {
            return null;
        }
        return $proof['serverTime'];
    }

    /** 校验缓存写在临时目录，读出时重新验签，手改文件没有用。 */
    private static function cachePath($ctx)
    {
        $key = implode("\n", array((string)self::cfg('baseUrl', ''), (string)self::cfg('appKey', ''), $ctx['licenseKey'], $ctx['domain'], $ctx['serverIp']));
        return rtrim(sys_get_temp_dir(), DIRECTORY_SEPARATOR) . DIRECTORY_SEPARATOR . 'authpro-' . substr(hash('sha256', $key), 0, 24) . '.json';
    }

    private static function readCache($cacheFile, $ctx, $publicKey)
    {
        if (!is_file($cacheFile)) {
            return null;
        }
        $entry = json_decode((string)@file_get_contents($cacheFile), true);
        if (!is_array($entry) || !isset($entry['body'], $entry['nonce'])) {
            return null;
        }
        $normalized = self::normalizeResult($entry['body']);
        if (!$normalized['ok']) {
            return null;
        }
        $serverTime = self::verifyLicenseProof($publicKey, $ctx['licenseKey'], (string)$entry['nonce'], $entry['body']);
        if ($serverTime === null) {
            return null;
        }
        $expireTs = isset($entry['body']['data']['expireTs']) && is_int($entry['body']['data']['expireTs']) ? $entry['body']['data']['expireTs'] : 0;
        return array(
            'savedAt' => isset($entry['savedAt']) ? (int)$entry['savedAt'] : 0,
            'body' => $entry['body'],
            'serverTime' => $serverTime,
            'expireTs' => $expireTs,
        );
    }

    private static function writeCache($cacheFile, $entry)
    {
        $tmp = $cacheFile . '.tmp';
        $old = umask(0077);
        $written = @file_put_contents($tmp, json_encode($entry));
        umask($old);
        if ($written !== false) {
            @rename($tmp, $cacheFile);
        }
    }

    private static function entryResult($entry, $flag)
    {
        $result = self::normalizeResult($entry['body']);
        $data = is_array($result['data']) ? $result['data'] : array();
        $data[$flag] = true;
        $result['data'] = $data;
        return $result;
    }

    private static function positiveOr($value, $fallback)
    {
        $number = (int)$value;
        return $number > 0 ? $number : $fallback;
    }

    private static function text($source, $key)
    {
        return isset($source[$key]) && is_string($source[$key]) ? $source[$key] : '';
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
            $status = (int)curl_getinfo($ch, CURLINFO_HTTP_CODE);
            curl_close($ch);
            if ($raw === false || $status === 0) {
                $raw = false;
            }
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
        // 连不上或响应不是 JSON 时返回 null，授权校验据此进入离线宽限；其它接口当成空结果。
        if ($raw === false || $raw === null) {
            return null;
        }
        $decoded = json_decode($raw === '' ? '{}' : $raw, true);
        return is_array($decoded) ? $decoded : null;
    }
}
