// Test-only support. string_join/repeat/split copied verbatim from pinned common.cpp.
#include "common.h"
#include "llama-impl.h"
#include <cstdarg>
#include <cstdio>
#include <cstdlib>
std::string string_join(const std::vector<std::string> & values, const std::string & separator) {
    std::ostringstream result;
    for (size_t i = 0; i < values.size(); ++i) {
        if (i > 0) {
            result << separator;
        }
        result << values[i];
    }
    return result.str();
}

std::string string_repeat(const std::string & str, size_t n) {
    if (n == 0) {
        return "";
    }

    std::string result;
    result.reserve(str.length() * n);

    for (size_t i = 0; i < n; ++i) {
        result += str;
    }

    return result;
}

// Test-only logging/abort hooks; no grammar or schema logic is replaced.
void ggml_abort(const char * file, int line, const char * fmt, ...) {
    std::fprintf(stderr, "%s:%d: ", file, line);
    va_list args; va_start(args, fmt); std::vfprintf(stderr, fmt, args); va_end(args);
    std::abort();
}
void llama_log_internal(ggml_log_level, const char * fmt, ...) {
    va_list args; va_start(args, fmt); std::vfprintf(stderr, fmt, args); va_end(args);
}

std::vector<std::string> string_split(const std::string & str, const std::string & delimiter) {
    std::vector<std::string> parts;
    size_t start = 0;
    size_t end = str.find(delimiter);

    while (end != std::string::npos) {
        parts.push_back(str.substr(start, end - start));
        start = end + delimiter.length();
        end = str.find(delimiter, start);
    }

    parts.push_back(str.substr(start));

    return parts;
}
