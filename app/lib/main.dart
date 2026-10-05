import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'windows_agent.dart';

void main() => runApp(const HomeProxyApp());

class HomeProxyApp extends StatelessWidget {
  const HomeProxyApp({super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
    debugShowCheckedModeBanner: false,
    theme: ThemeData(
      colorScheme: ColorScheme.fromSeed(
        seedColor: const Color(0xffb8dc65),
        brightness: Brightness.dark,
      ),
      scaffoldBackgroundColor: const Color(0xff111614),
      useMaterial3: true,
    ),
    home: defaultTargetPlatform == TargetPlatform.windows
        ? const WindowsAgentScreen()
        : const AgentScreen(),
  );
}

class AgentScreen extends StatefulWidget {
  const AgentScreen({super.key});
  @override
  State<AgentScreen> createState() => _AgentScreenState();
}

class _AgentScreenState extends State<AgentScreen> {
  static const channel = MethodChannel('homeproxy/agent');
  final address = TextEditingController();
  final id = TextEditingController(text: 'telefon');
  final token = TextEditingController();
  final fingerprint = TextEditingController();
  final form = GlobalKey<FormState>();
  Timer? timer;
  String status = 'Durduruldu';
  String? error;
  bool busy = false;
  bool insecure = false;
  bool loading = true;
  Map<String, Object> get settings => {
    'address': address.text.trim(),
    'id': id.text.trim(),
    'token': token.text,
    'fingerprint': fingerprint.text.trim(),
    'insecure': insecure,
  };

  Future<void> loadSettings() async {
    try {
      final saved = await channel.invokeMapMethod<String, dynamic>(
        'loadSettings',
      );
      if (!mounted) return;
      if (saved != null && saved.isNotEmpty) {
        address.text = saved['address'] as String? ?? '';
        id.text = saved['id'] as String? ?? 'telefon';
        token.text = saved['token'] as String? ?? '';
        fingerprint.text = saved['fingerprint'] as String? ?? '';
        insecure = saved['insecure'] == true;
      }
    } on PlatformException catch (e) {
      if (mounted) error = e.message ?? 'Ayarlar okunamadı';
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> changeTLS(bool value) async {
    if (!value) {
      setState(() => insecure = false);
      return;
    }
    final accepted = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Sunucu kimliği doğrulanmayacak'),
        content: const Text(
          'Trafik şifreli kalır, ancak sahte sunucu token’ını ele geçirebilir. Yalnız riski kabul ediyorsan devam et.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Vazgeç'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Riski kabul et'),
          ),
        ],
      ),
    );
    if (mounted && accepted == true) setState(() => insecure = true);
  }

  @override
  void initState() {
    super.initState();
    timer = Timer.periodic(const Duration(seconds: 1), (_) => refresh());
    refresh();
    loadSettings();
  }

  Future<void> refresh() async {
    try {
      final value = await channel.invokeMethod<String>('status');
      if (mounted) setState(() => status = value ?? 'Durduruldu');
    } on PlatformException catch (_) {
      if (mounted) setState(() => error = 'Android servisine erişilemiyor');
    }
  }

  Future<void> command(bool start) async {
    if (start && !form.currentState!.validate()) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      if (start) await channel.invokeMethod('saveSettings', settings);
      await channel.invokeMethod(
        start ? 'start' : 'stop',
        start ? settings : null,
      );
      await refresh();
    } on PlatformException catch (e) {
      if (mounted) setState(() => error = e.message ?? 'İşlem başarısız');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  void dispose() {
    timer?.cancel();
    for (final controller in [address, id, token, fingerprint]) {
      controller.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('HOMEPROXY')),
    body: loading
        ? const Center(child: CircularProgressIndicator())
        : SafeArea(
            child: ListView(
              padding: const EdgeInsets.all(24),
              children: [
                const Text(
                  'TELEFON AGENT',
                  style: TextStyle(letterSpacing: 3, color: Color(0xffb8dc65)),
                ),
                const SizedBox(height: 12),
                const Text(
                  'İnternet çıkışın.\nKontrol sende.',
                  style: TextStyle(fontSize: 34, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 24),
                Semantics(
                  liveRegion: true,
                  child: Card(
                    child: Padding(
                      padding: const EdgeInsets.all(20),
                      child: Row(
                        children: [
                          Icon(status == 'Bağlı' ? Icons.link : Icons.link_off),
                          const SizedBox(width: 12),
                          Expanded(child: Text(status)),
                        ],
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 20),
                if (insecure)
                  const Padding(
                    padding: EdgeInsets.only(bottom: 16),
                    child: Text(
                      'Uyarı: TLS sunucu kimliği doğrulanmıyor.',
                      style: TextStyle(color: Colors.orange),
                    ),
                  ),
                Form(
                  key: form,
                  child: Column(
                    children: [
                      field(
                        address,
                        'Sunucu adresi',
                        hint: 'proxy.example.com:4433',
                      ),
                      field(id, 'Agent ID'),
                      field(token, 'Token', secret: true),
                      SwitchListTile(
                        title: const Text('TLS doğrulamasını atla (insecure)'),
                        subtitle: const Text(
                          'Sunucu kimliği doğrulanmaz. Değişiklik sonraki bağlantıda uygulanır.',
                        ),
                        value: insecure,
                        onChanged: busy ? null : changeTLS,
                      ),
                      if (!insecure)
                        field(
                          fingerprint,
                          'Sertifika SHA-256',
                          hint: '64 karakterlik parmak izi',
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
                FilledButton(
                  onPressed: busy ? null : () => command(true),
                  child: const Text('Bağlantıyı başlat'),
                ),
                const SizedBox(height: 8),
                OutlinedButton(
                  onPressed: busy ? null : () => command(false),
                  child: const Text('Durdur'),
                ),
                const SizedBox(height: 24),
                const Text(
                  'Yalnız sunucudan gelen proxy trafiği taşınır. VPN profili oluşturulmaz. Ayrı dış IP için mobil veri kullan.',
                  style: TextStyle(color: Colors.white60),
                ),
                const SizedBox(height: 12),
                const Text(
                  'Ayarlar cihazda şifreli saklanır. Açılışta bağlantı otomatik başlamaz. HyperOS pil kısıtları arka plan bağlantısını durdurabilir.',
                  style: TextStyle(color: Colors.white60),
                ),
              ],
            ),
          ),
  );
  Widget field(
    TextEditingController controller,
    String label, {
    String? hint,
    bool secret = false,
  }) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: TextFormField(
      controller: controller,
      obscureText: secret,
      autocorrect: false,
      enableSuggestions: false,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        border: const OutlineInputBorder(),
      ),
      validator: (value) =>
          value == null || value.trim().isEmpty ? '$label gerekli' : null,
    ),
  );
}
