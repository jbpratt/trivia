import requests
import json
import time
from typing import Any
from typing import Dict

question_dict = dict()

KEY_CATEGORY = "category"
KEY_TYPE = "type"
KEY_DIFFICULTY = "difficulty"
KEY_QUESTION = "question"
KEY_ANSWER_CORRECT = "correct_answer"
KEY_ANSWER_WRONG = "incorrect_answers"

# requesting more than 20 questions at once leads to categories with very few
# questions like category 30 returning no questions at all
QUERY_SIZE = 20


def get_token() -> str:
    url_token = "https://opentdb.com/api_token.php?command=request"
    res = requests.get(url_token)
    site_json = json.loads(res.text)
    assert "token" in site_json
    return site_json["token"]


def get_targets():
    url = "https://opentdb.com/api_count_global.php"
    res = requests.get(url)
    site_json = json.loads(res.text)
    target_dict = dict()
    for entry in site_json["categories"]:
        target_dict[entry] = site_json["categories"][entry]["total_num_of_questions"]
    return target_dict


def create_dict(message) -> Dict[str, Any]:
    new_dict = dict()
    new_dict[KEY_CATEGORY] = message[KEY_CATEGORY]
    new_dict[KEY_TYPE] = message[KEY_TYPE]
    new_dict[KEY_DIFFICULTY] = message[KEY_DIFFICULTY]
    new_dict[KEY_QUESTION] = message[KEY_QUESTION]
    new_dict[KEY_ANSWER_CORRECT] = message[KEY_ANSWER_CORRECT]
    new_dict[KEY_ANSWER_WRONG] = message[KEY_ANSWER_WRONG]
    return new_dict


def get_questions_from_category(
    category: str, target_number: int, query_token: str
) -> None:
    counter_queries = 0
    while counter_queries < (target_number / QUERY_SIZE):
        api_request = f"https://opentdb.com/api.php?amount={QUERY_SIZE}&category={category}&token={query_token}"
        res = requests.get(api_request)
        site_json = json.loads(res.text)
        for message in site_json["results"]:
            message_entry = create_dict(message)
            if message_entry[KEY_QUESTION] not in question_dict:
                question_dict[message_entry[KEY_QUESTION]] = message_entry
        counter_queries += 1
        time.sleep(1)


def get_all_questions(token: str) -> None:
    targets = get_targets()
    for key in targets:
        get_questions_from_category(key, targets[key], token)


def main() -> int:
    counter_crawling_rounds = 0
    while True:
        query_token = get_token()  # refresh token for each crawling round
        count_questions_previous = len(question_dict)
        get_all_questions(query_token)
        print(
            f"{counter_crawling_rounds}: Added Questions: {len(question_dict) - count_questions_previous}"
        )
        with open("crawled_data.json", "w", encoding="utf-8") as f:
            json.dump(question_dict, f, ensure_ascii=False, indent=4)
        print("Writing to file done")
        counter_crawling_rounds += 1

    return 0


if __name__ == "__main__":
    exit(main())
