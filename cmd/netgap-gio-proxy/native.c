//go:build linux && cgo && gio

#include "native.h"

const char *ng_backend(void) {
    GProxyResolver *resolver = g_proxy_resolver_get_default();
    return resolver ? G_OBJECT_TYPE_NAME(resolver) : NULL;
}

static gint compare_keys(gconstpointer a, gconstpointer b) {
    return g_strcmp0(*(const char * const *)a, *(const char * const *)b);
}

static void snapshot_settings(GSettingsSchema *schema, GString *out) {
    GSettings *settings = g_settings_new_full(schema, NULL, NULL);
    char **keys = g_settings_schema_list_keys(schema);
    /* GStrv ordering is not promised; sorting is required for stable hashes. */
    qsort(keys, g_strv_length(keys), sizeof(char *), compare_keys);
    for (int i=0; keys[i]; i++) {
        GVariant *value = g_settings_get_value(settings, keys[i]);
        char *text = g_variant_print(value, TRUE);
        g_string_append_printf(out, "%s %s %s\n", g_settings_schema_get_id(schema), keys[i], text);
        g_free(text); g_variant_unref(value);
    }
    g_strfreev(keys);
    char **children = g_settings_schema_list_children(schema);
    qsort(children, g_strv_length(children), sizeof(char *), compare_keys);
    for (int i=0; children[i]; i++) {
        GSettings *child_settings = g_settings_get_child(settings, children[i]);
        GSettingsSchema *child = NULL;
        g_object_get(child_settings, "settings-schema", &child, NULL);
        snapshot_settings(child, out); g_settings_schema_unref(child);
        g_object_unref(child_settings);
    }
    g_strfreev(children); g_object_unref(settings);
}

char *ng_snapshot(int *configured) {
    *configured = 1;
    if (g_strcmp0(ng_backend(), "GProxyResolverGnome") != 0) return g_strdup("");
    GSettingsSchemaSource *source = g_settings_schema_source_get_default();
    GSettingsSchema *schema = source ? g_settings_schema_source_lookup(source, "org.gnome.system.proxy", TRUE) : NULL;
    if (!schema) return NULL;
    GSettings *settings = g_settings_new_full(schema, NULL, NULL);
    char *mode = g_settings_get_string(settings, "mode");
    *configured = g_strcmp0(mode,"none") != 0;
    GString *out = g_string_new(NULL);
    snapshot_settings(schema,out);
    g_free(mode); g_object_unref(settings); g_settings_schema_unref(schema);
    return g_string_free(out,FALSE);
}

char **ng_lookup(const char *target, char **error) {
    GError *failure = NULL;
    char **routes = g_proxy_resolver_lookup(g_proxy_resolver_get_default(),target,NULL,&failure);
    if (failure) { *error=g_strdup(failure->message); g_error_free(failure); }
    return routes;
}
