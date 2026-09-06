#include "json-schema-to-grammar.h"
#include "llama-grammar.h"
#include <fstream>
#include <iostream>
#include <algorithm>
int main(int argc, char ** argv) {
    if (argc < 3) return 2;
    std::ifstream input(argv[1]);
    nlohmann::ordered_json schema; input >> schema;
    std::string gbnf = json_schema_to_grammar(schema);
    if (gbnf != json_schema_to_grammar(schema, true)) return 5;
    std::ofstream output(argv[2]); output << gbnf; output.close();
    if (argc == 3) return 0;
    std::ifstream cases_in(argv[3]);
    nlohmann::ordered_json cases; cases_in >> cases;
    int failures = 0;
    for (const auto & test : cases) {
        auto * grammar = llama_grammar_init_impl(nullptr, gbnf.c_str(), "root", false, nullptr, 0, nullptr, 0);
        if (!grammar) return 3;
        std::string value = test.at("json").get<std::string>();
        for (unsigned char c : value) {
            if (c > 127) return 4; // proof inputs are deliberately ASCII only
            llama_grammar_accept(grammar, c);
            if (grammar->stacks.empty()) break;
        }
        bool accepted = std::any_of(grammar->stacks.begin(), grammar->stacks.end(), [](const llama_grammar_stack & s) { return s.empty(); });
        bool expected = test.at("accepted").get<bool>();
        std::cout << (accepted == expected ? "PASS " : "FAIL ") << test.at("name").get<std::string>()
                  << " accepted=" << accepted << " expected=" << expected << "\n";
        failures += accepted != expected;
        llama_grammar_free_impl(grammar);
    }
    return failures ? 1 : 0;
}
