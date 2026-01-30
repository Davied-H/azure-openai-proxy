import os

os.environ['NO_PROXY'] = 'localhost,127.0.0.1'

from openai import OpenAI

# 配置
BASE_URL = "http://localhost:3000/v1"
API_KEY = "sk-your-api-key"
MODEL = "gpt-4"
PROMPT = "你好，请用一句话介绍自己"


def test_blocking(client: OpenAI):
    """测试阻塞请求"""
    print("=" * 50)
    print("测试阻塞请求")
    print("=" * 50)

    response = client.chat.completions.create(
        model=MODEL,
        messages=[{"role": "user", "content": PROMPT}],
        stream=False
    )

    print(f"模型: {response.model}")
    print(f"回复: {response.choices[0].message.content}")
    print(f"Token 使用: {response.usage}")
    print()


def test_stream(client: OpenAI):
    """测试流式请求"""
    print("=" * 50)
    print("测试流式请求")
    print("=" * 50)

    stream = client.chat.completions.create(
        model=MODEL,
        messages=[{"role": "user", "content": PROMPT}],
        stream=True
    )

    print("回复: ", end="", flush=True)
    for chunk in stream:
        if chunk.choices and chunk.choices[0].delta.content:
            print(chunk.choices[0].delta.content, end="", flush=True)
    print("\n")


if __name__ == '__main__':
    client = OpenAI(base_url=BASE_URL, api_key=API_KEY)

    print(f"基础 URL: {BASE_URL}")
    print(f"模型: {MODEL}")
    print(f"提示词: {PROMPT}")
    print()

    test_blocking(client)
    test_stream(client)
