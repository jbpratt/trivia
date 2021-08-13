import json
import time
import random
import requests

from typing import Set


def get_snippet(string: str, start: str, end: str) -> str:
    begin = string.find(start) + len(start)
    endp = string[begin:].find(end)
    return string[begin : begin + endp]


def main() -> int:
    format_url = "https://www.millionairedb.com/_nuxt/static/1616645132/questions/{slug}/payload.js"

    all_urls = set()
    all_urls.add("1757s-battle-of-plassey-was-fought-in-which-country")
    visited_urls: Set[str] = set()

    while True:
        time.sleep(1)
        slug = random.choice(list(all_urls.difference(visited_urls)))
        visited_urls.add(slug)
        r = requests.get(format_url.format(slug=slug))

        for url_prefix in [
            'next:{slug:"',
            'next1:{slug:"',
            'next2:{slug:"',
            'prev2:{slug:"',
            'prev1:{slug:"',
            'prev:{slug:"',
        ]:
            if url_prefix in r.text:
                snippet = get_snippet(r.text, url_prefix, '"')
                if len(snippet) > 2:
                    all_urls.add(snippet)

        if "ERROR" in r.text or "choices" not in r.text:
            continue

        question = get_snippet(r.text, 'question:"', '"')
        choices = get_snippet(r.text, ",choices:[", "]").split(",")
        parameters = get_snippet(r.text, "}}(", ")))").split(",")

        real_choices = []
        answer = ""
        for choice in choices:
            if '"' in choice:
                real_choices.append(choice)
                continue

            index = {"a": 0, "b": 1, "c": 2, "d": 3}[choice]
            answer = parameters[index]
            real_choices.append(answer)

        real_choices = list(map(lambda x: x.replace('"', ""), real_choices))
        if real_choices:
            print(
                json.dumps(
                    {"question": question, "choices": real_choices, "answer": answer}
                )
            )

    return 0


if __name__ == "__main__":
    exit(main())
