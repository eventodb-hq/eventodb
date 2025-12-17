Look at the current code base
It is a event sourcing database built on top of Postgres. 
It's using store procedures and some views and a lot of bash scripts to do this. 



Now, I kinda like it. 
But I would like to put a nice go long REST API on top of it. 
I do not want to deal with um Postgres users. 
I do not want uh to have to deal with Postgres credentials. 
And I don't want to deal with um connection counting and um pools and all other very special things. 
So I imagine this to be a simple golang  REST API. 
It would use server cent events for real time features. 
And it would hide away the actual implementation. 

And this would allow us to support namespace. 
And we could also support um test stores that are based on SQLite and uh run purely in memory. 
Another thing is we could have uh statistics for the Postgres database that we keep locally so that we by just calling the local SQLI database for recent data. 
So there are quite some interesting things we can do if we abstract away the Postgres and don't let uh consumers. 

This means also that the consumers would not need to bring apostas. client anymore, but could use just a random HTTP library. 
I understand there might be some Performance loss due to the extra abstraction, duh but it makes things actually simpler. 

What do you think about this idea? 