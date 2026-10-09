import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class WindowsAgentScreen extends StatefulWidget {
  const WindowsAgentScreen({super.key});
  @override
  State<WindowsAgentScreen> createState() => _WindowsAgentScreenState();
}

class _WindowsAgentScreenState extends State<WindowsAgentScreen>
    with WidgetsBindingObserver {
  static const channel = MethodChannel('homeproxy/windows');
  final address = TextEditingController();
  final id = TextEditingController(text: 'ev-pc');
  final token = TextEditingController();
  final fingerprint = TextEditingController();
  final form = GlobalKey<FormState>();
  Timer? timer;
  bool visible = true;
  int pollGeneration = 0;
  String service = 'loading';
  String status = 'Durduruldu';
  String? error;
  bool busy = false, polling = false, restored = false;
  bool refreshAfterCommand = false;
  bool configured = false, enabled = false, insecure = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    visible =
        WidgetsBinding.instance.lifecycleState == null ||
        WidgetsBinding.instance.lifecycleState == AppLifecycleState.resumed;
    startPolling();
  }

  void startPolling() {
    if (!visible) return;
    refresh();
    timer = Timer.periodic(const Duration(seconds: 2), (_) {
      if (!busy && !polling) refresh();
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    final next = state == AppLifecycleState.resumed;
    if (visible == next) return;
    visible = next;
    pollGeneration++;
    timer?.cancel();
    timer = null;
    if (visible && !polling) startPolling();
  }

  Future<Map<String, dynamic>> request(
    String op, [
    Map<String, Object>? config,
  ]) async {
    final raw = await channel.invokeMethod<String>(
      'request',
      jsonEncode({'version': 1, 'op': op, 'config': ?config}),
    );
    try {
      final response = jsonDecode(raw ?? '') as Map<String, dynamic>;
      if (response['version'] != 1 || response['ok'] is! bool) {
        throw const FormatException();
      }
      if (response['ok'] != true) {
        throw PlatformException(
          code: 'ipc',
          message: response['error'] as String? ?? 'Servis işlemi başarısız',
        );
      }
      final config = response['config'];
      if (response['status'] is! String ||
          (config != null &&
              (config is! Map<String, dynamic> ||
                  config['address'] is! String ||
                  config['id'] is! String ||
                  config['fingerprint'] is! String ||
                  config['enabled'] is! bool ||
                  config['insecure'] is! bool))) {
        throw const FormatException();
      }
      return response;
    } on FormatException {
      throw PlatformException(
        code: 'ipc',
        message: 'Geçersiz servis yanıtı; paketi ve servisi kontrol et.',
      );
    } on TypeError {
      throw PlatformException(
        code: 'ipc',
        message: 'Geçersiz servis yanıtı; paketi ve servisi kontrol et.',
      );
    }
  }

  void applyResponse(Map<String, dynamic> response, {bool restore = false}) {
    status = response['status'] as String? ?? 'Durduruldu';
    final config = response['config'] as Map<String, dynamic>?;
    configured = config != null;
    enabled = config?['enabled'] == true;
    if (restore && config != null) {
      address.text = config['address'] as String? ?? '';
      id.text = config['id'] as String? ?? 'ev-pc';
      fingerprint.text = config['fingerprint'] as String? ?? '';
      insecure = config['insecure'] == true;
      token.clear(); // Backend never returns stored token.
    }
  }

  Object pollView() => (
    service,
    status,
    error,
    configured,
    enabled,
    insecure,
    address.text,
    id.text,
    fingerprint.text,
    token.text,
  );

  Future<void> refresh({
    bool clearError = false,
    bool commandRefresh = false,
  }) async {
    if (!visible) return;
    if (busy && !commandRefresh) {
      refreshAfterCommand = true;
      return;
    }
    if (polling) return;
    refreshAfterCommand = false;
    polling = true;
    final generation = pollGeneration;
    final before = pollView();
    try {
      final state = await channel.invokeMethod<String>('serviceStatus');
      if (!mounted || !visible || generation != pollGeneration) return;
      final nextService = state ?? 'unknown';
      if (nextService == 'running') {
        final response = await request('status');
        if (!mounted || !visible || generation != pollGeneration) return;
        applyResponse(response, restore: !restored);
        restored = true;
      } else {
        // A restarted service must restore redacted settings, but never clear
        // unsaved edits during routine polling while it stays running.
        restored = false;
      }
      service = nextService;
      if (clearError) error = null;
    } on PlatformException catch (e) {
      if (!mounted || !visible || generation != pollGeneration) return;
      error = e.message ?? 'Windows servisine erişilemiyor';
      if (service == 'loading') service = 'unknown';
    } finally {
      polling = false;
      if (mounted && visible) {
        if (generation == pollGeneration) {
          if (pollView() != before) setState(() {});
        } else {
          startPolling();
        }
      }
    }
  }

  Future<bool> confirm(String title, String message, String action) async =>
      await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: Text(title),
          content: Text(message),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Vazgeç'),
            ),
            TextButton(
              onPressed: () => Navigator.pop(context, true),
              child: Text(action),
            ),
          ],
        ),
      ) ==
      true;

  Future<void> changeTLS(bool value) async {
    if (!value ||
        await confirm(
          'Sunucu kimliği doğrulanmayacak',
          'Sunucu kimliği doğrulanmaz; saldırgan kimlik doğrulamasını ve trafiği ele geçirebilir. Güvenilir SHA-256 parmak izini tercih et.',
          'Riski kabul et',
        )) {
      if (mounted) setState(() => insecure = value);
    }
  }

  Future<void> run(Future<void> Function() action) async {
    if (busy || polling) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await action();
    } on PlatformException catch (e) {
      if (mounted) setState(() => error = e.message ?? 'İşlem başarısız');
    } finally {
      if (mounted) {
        setState(() => busy = false);
        if (visible && refreshAfterCommand) await refresh();
      }
    }
  }

  Future<void> save(bool connect) async {
    if (!form.currentState!.validate()) return;
    await run(() async {
      // Full replacement. Saving alone preserves persisted tunnel intent.
      final response = await request('set_config', {
        'address': address.text.trim(),
        'id': id.text.trim(),
        'token': token.text,
        'fingerprint': fingerprint.text.trim(),
        'insecure': insecure,
        'enabled': connect || enabled,
      });
      if (!mounted) return;
      setState(() {
        applyResponse(response);
        token.clear();
      });
    });
  }

  Future<void> tunnel(String op) => run(() async {
    final response = await request(op);
    if (mounted) setState(() => applyResponse(response));
  });

  Future<void> scm(String action) async {
    const messages = {
      'install': 'HomeProxyAgent otomatik açılış servisi kurulacak. Bu Windows kullanıcısı yetkilendirilecek. Kurulum şimdi servisi başlatmaz. Eski console/VBScript agent otomatik kapatılmaz.',
      'uninstall': 'Durdurulmuş HomeProxyAgent ve kalıcı servis dosyası kaldırılacak. Şifreli ayarlar korunur. Yeniden kurulum UAC ister.',
      'start': 'Windows servisi başlatılacak. Kaydedilmiş bağlantı tercihi açıksa tünel de başlar.',
      'stop': 'Windows servisi ve tünel duracak; GUI ayarlara erişemez. Bu işlem kalıcı tünel kapatma değildir. Yalnız tüneli kapatmak için Bağlantıyı kes kullan.',
    };
    if (!await confirm(
      'Windows servisi: $action',
      messages[action]!,
      'Onayla (UAC)',
    )) {
      return;
    }
    if (!mounted) return;
    await run(() async {
      await channel.invokeMethod<String>('serviceCommand', action);
      // The SCM call has finished; keep this command's explicit refresh.
      await refresh(clearError: true, commandRefresh: true);
    });
  }

  String? validate(String label, String? input) {
    final value = input ?? '';
    if (label == 'Token') {
      if (value.isEmpty && configured) return null;
      final size = utf8.encode(value).length;
      return size < 16 || size > 4096 ? 'Token 16–4096 bayt olmalı' : null;
    }
    final trimmed = value.trim();
    if (label == 'Sunucu adresi') {
      final match = RegExp(r'^(\[[^\]\s]+\]|[^:\s]+):(\d+)$')
          .firstMatch(trimmed);
      final port = int.tryParse(match?.group(2) ?? '') ?? 0;
      return match == null || port < 1 || port > 65535
          ? 'Adres host:port olmalı (1–65535)'
          : null;
    }
    if (label == 'Agent ID') {
      return trimmed.isEmpty ||
              utf8.encode(trimmed).length > 64 ||
              RegExp(r'[\x00-\x1f\x7f/\\]').hasMatch(trimmed)
          ? 'ID 1–64 bayt; kontrol karakteri veya eğik çizgi içeremez'
          : null;
    }
    return RegExp(r'^[0-9a-fA-F]{64}$').hasMatch(trimmed)
        ? null
        : 'Parmak izi 64 hex karakter olmalı';
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    timer?.cancel();
    for (final controller in [address, id, token, fingerprint]) {
      controller.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final running = service == 'running';
    final locked = busy || polling;
    final serviceLabel = switch (service) {
      'loading' => 'Servis durumu okunuyor…',
      'missing' => 'Windows servisi kurulu değil',
      'stopped' => 'Windows servisi durdurulmuş',
      'pending' => 'Windows servisi geçiş halinde; bekle',
      'running' => 'Windows servisi çalışıyor',
      _ => 'Windows servis durumu okunamadı',
    };
    return Scaffold(
      appBar: AppBar(title: const Text('HOMEPROXY')),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 720),
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                const Text(
                  'WINDOWS AGENT',
                  style: TextStyle(letterSpacing: 3, color: Color(0xffb8dc65)),
                ),
                const SizedBox(height: 12),
                const Text(
                  'İnternet çıkışın.\nKontrol sende.',
                  style: TextStyle(fontSize: 34, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 20),
                Semantics(
                  liveRegion: true,
                  child: Card(
                    child: Padding(
                      padding: const EdgeInsets.all(20),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(serviceLabel),
                          if (running) Text('Tünel: $status'),
                          if (running)
                            Text(
                              configured
                                  ? 'Kalıcı bağlantı tercihi: ${enabled ? "açık" : "kapalı"}'
                                  : 'İlk kurulum: bağlantı ayarlarını kaydet',
                            ),
                        ],
                      ),
                    ),
                  ),
                ),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    if (service == 'missing')
                      OutlinedButton(
                        onPressed: locked ? null : () => scm('install'),
                        child: const Text('Servisi kur'),
                      ),
                    if (service == 'stopped')
                      OutlinedButton(
                        onPressed: locked ? null : () => scm('start'),
                        child: const Text('Servisi başlat'),
                      ),
                    if (service == 'stopped')
                      OutlinedButton(
                        onPressed: locked ? null : () => scm('uninstall'),
                        child: const Text('Servisi kaldır'),
                      ),
                    if (running)
                      OutlinedButton(
                        onPressed: locked ? null : () => scm('stop'),
                        child: const Text('Windows servisini durdur'),
                      ),
                    TextButton(
                      onPressed: locked
                          ? null
                          : () => refresh(clearError: true),
                      child: const Text('Yenile'),
                    ),
                  ],
                ),
                const SizedBox(height: 20),
                if (!running)
                  const Text(
                    'Ayarlar ve tünel komutları için servisi kur ve ayrı onayla başlat. GUI kendi tünelini açmaz.',
                  ),
                if (insecure)
                  const Text(
                    'Uyarı: TLS sunucu kimliği doğrulanmıyor.',
                    style: TextStyle(color: Colors.orange),
                  ),
                Form(
                  key: form,
                  child: Column(
                    children: [
                      field(
                        address,
                        'Sunucu adresi',
                        enabled: running && !locked,
                        hint: 'proxy.example.com:4433',
                      ),
                      field(id, 'Agent ID', enabled: running && !locked),
                      field(
                        token,
                        'Token',
                        enabled: running && !locked,
                        secret: true,
                        hint: configured
                            ? 'Boş bırak: kayıtlı token korunur'
                            : 'İlk kurulum: en az 16 bayt',
                      ),
                      SwitchListTile(
                        title: const Text('TLS doğrulamasını atla (insecure)'),
                        value: insecure,
                        onChanged: running && !locked ? changeTLS : null,
                      ),
                      if (!insecure)
                        field(
                          fingerprint,
                          'Sertifika SHA-256',
                          enabled: running && !locked,
                          hint: 'Güvenilir kaynaktan 64 hex karakter',
                        ),
                    ],
                  ),
                ),
                if (error != null)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 16),
                    child: Text(
                      error!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                      ),
                    ),
                  ),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    FilledButton(
                      onPressed: running && !locked ? () => save(true) : null,
                      child: const Text('Kaydet ve bağlan'),
                    ),
                    OutlinedButton(
                      onPressed: running && !locked ? () => save(false) : null,
                      child: const Text('Ayarları kaydet'),
                    ),
                    if (configured)
                      OutlinedButton(
                        onPressed: running && !locked
                            ? () => tunnel('connect')
                            : null,
                        child: const Text('Kayıtlı ayarlarla bağlan'),
                      ),
                    OutlinedButton(
                      onPressed: running && !locked && configured
                          ? () => tunnel('disconnect')
                          : null,
                      child: const Text('Bağlantıyı kes'),
                    ),
                  ],
                ),
                if (busy)
                  const Padding(
                    padding: EdgeInsets.all(12),
                    child: LinearProgressIndicator(),
                  ),
                const SizedBox(height: 24),
                const Text(
                  'Bağlantıyı kes: tüneli kalıcı kapatır, servis çalışır. Servis otomatik açılır; kayıtlı bağlantı tercihi yeniden uygulanır. Ayarlar servis tarafından DPAPI ile şifrelenir; token geri okunmaz. VPN profili oluşturulmaz.',
                  style: TextStyle(color: Colors.white60),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget field(
    TextEditingController controller,
    String label, {
    required bool enabled,
    bool secret = false,
    String? hint,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: TextFormField(
      controller: controller,
      enabled: enabled,
      obscureText: secret,
      autocorrect: false,
      enableSuggestions: false,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        border: const OutlineInputBorder(),
      ),
      validator: (value) => validate(label, value),
    ),
  );
}
