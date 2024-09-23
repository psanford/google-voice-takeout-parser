# Google Voice Takeout Parser

This is a simple tool for parsing Google Voice takeout html files to either newline delimited json or sqlite.

```
google-voice-takeout-parser [-format=<json|sqlite>]
```

By default, the tool outputs in JSON format. Use the `-format` flag to specify SQLite output.

## Input

The tool expects HTML files from a Google Voice takeout in the current directory. It will process all `.html` files found.

## Output

### JSON Format

When using JSON output, each conversation is printed as a JSON object to stdout.

```
{"type":"missed_call","participants":{"Dwigt Rortugal":"+66666"},"timestamp":"2009-09-17T17:26:41-07:00","source_file":"missedcall.html"}
{"type":"chat","participants":{"Me":"+2222","Mike Truk":"+8888","Tony Smehrik":"+333"},"timestamp":"2024-05-22T21:48:32.703-07:00","messages":[{"timestamp":"2024-05-22T21:48:32.703-07:00","sender":"Mike Truk","sender_number":"+8888","content":"","images":["Group Conversation - 2024-05-23T04_48_32Z-1-1","Group Conversation - 2024-05-23T04_48_32Z-1-2"]},{"timestamp":"2024-05-22T21:49:25.704-07:00","sender":"Me","sender_number":"+2222","content":"","images":["Group Conversation - 2024-05-23T04_48_32Z-2-1"]},{"timestamp":"2024-05-22T21:49:33.853-07:00","sender":"Me","sender_number":"+2222","content":"","images":["Group Conversation - 2024-05-23T04_48_32Z-3-1"]},{"timestamp":"2024-05-22T21:50:42.475-07:00","sender":"Mike Truk","sender_number":"+8888","content":"Hahahaha"},{"timestamp":"2024-05-22T21:51:10.663-07:00","sender":"Mike Truk","sender_number":"+8888","content":"Maybe this is your sign to get a hornet-skyscraper Peter"},{"timestamp":"2024-05-22T21:54:15.125-07:00","sender":"Tony Smehrik","sender_number":"+333","content":"Hahaha I love all of these"}],"source_file":"mms.html"}
{"type":"chat","participants":{"Me":"+2222","Tony Smehrik":"+333"},"timestamp":"2022-06-30T18:06:39.894-07:00","messages":[{"timestamp":"2022-06-30T18:06:39.894-07:00","sender":"Me","sender_number":"+2222","content":"doing just fine. I moved to Florida"},{"timestamp":"2022-06-30T18:06:46.025-07:00","sender":"Me","sender_number":"+2222","content":"MMS Sent","images":["Tony Smehrik - Text - 2022-07-01T01_06_39Z-2-1"]},{"timestamp":"2022-06-30T18:07:09.468-07:00","sender":"Tony Smehrik","sender_number":"+333","content":"💚"},{"timestamp":"2022-06-30T18:07:24.594-07:00","sender":"Tony Smehrik","sender_number":"+333","content":"all that space"},{"timestamp":"2022-06-30T18:07:28.19-07:00","sender":"Tony Smehrik","sender_number":"+333","content":"Thank you 🙏"}],"source_file":"sms.html"}
{"type":"chat","participants":{"Me":"+2222","Sillio Sanford":""},"timestamp":"2023-08-21T17:52:44.104-07:00","messages":[{"timestamp":"2023-08-21T17:52:44.104-07:00","sender":"Me","sender_number":"+2222","content":"Hey ya"},{"timestamp":"2023-08-21T18:02:19.924-07:00","sender":"Me","sender_number":"+2222","content":"How are you?"},{"timestamp":"2023-08-21T18:02:49.957-07:00","sender":"Me","sender_number":"+2222","content":"Apple","images":["Sillio Sanford - Text - 2023-08-22T00_52_44Z-3-1"]},{"timestamp":"2023-08-21T18:07:34.456-07:00","sender":"Me","sender_number":"+2222","content":"Just text"},{"timestamp":"2023-08-21T18:08:09.84-07:00","sender":"Me","sender_number":"+2222","content":"MMS Sent","images":["Sillio Sanford - Text - 2023-08-22T00_52_44Z-5-1"]},{"timestamp":"2023-08-21T21:12:17.519-07:00","sender":"Me","sender_number":"+2222","content":"Hey"}],"source_file":"sms2.html"}
{"type":"voicemail","participants":{"Sleve Mcdichael":"+11111111111"},"timestamp":"2018-07-23T09:23:31-07:00","duration":"00:00:18","transcript":"Hi Peter, this is Sleve Mcdichael. I'm the manager. I believe you have internet. I just have some quick questions for you. Thank you.","source_file":"voicemail.html"}
```

### SQLite Format

When using SQLite output, the tool creates a `conversations.db` file with the following schema:

- `conversations`: Stores overall conversation data
- `participants`: Stores participant information for each conversation
- `messages`: Stores individual messages within conversations
- `images`: Stores information about image attachments in messages
