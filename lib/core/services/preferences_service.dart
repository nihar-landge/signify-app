import 'package:shared_preferences/shared_preferences.dart';

/// Service for managing user preferences including sign language mode.
class PreferencesService {
  static const String _signLanguageKey = 'signLanguagePreference';
  static const String _userIdKey = 'userId';

  static PreferencesService? _instance;
  static SharedPreferences? _prefs;

  PreferencesService._();

  /// Get singleton instance of PreferencesService.
  static Future<PreferencesService> getInstance() async {
    if (_instance == null) {
      _instance = PreferencesService._();
      _prefs = await SharedPreferences.getInstance();
    }
    return _instance!;
  }

  /// Get current sign language preference: "ASL" or "ISL".
  String get signLanguage => _prefs?.getString(_signLanguageKey) ?? 'ASL';

  /// Set sign language preference.
  Future<void> setSignLanguage(String language) async {
    await _prefs?.setString(_signLanguageKey, language);
  }

  bool get isISL => signLanguage == 'ISL';
  bool get isASL => signLanguage == 'ASL';

  /// Get or generate a persistent unique user ID
  String get userId {
    String? id = _prefs?.getString(_userIdKey);
    if (id == null) {
      id = 'user_${DateTime.now().millisecondsSinceEpoch}';
      _prefs?.setString(_userIdKey, id);
    }
    return id;
  }

  /// Expose raw prefs for use by other services
  SharedPreferences? get prefs => _prefs;
}
